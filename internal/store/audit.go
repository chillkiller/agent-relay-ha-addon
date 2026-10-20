// Package store — Audit-Log (P3#4): append-only Event-Kette nach dem
// Paperclip-Audit-Muster ("governable work needs receipts") und der
// buzz-DNA "answers with receipts": Jedes relevante Relay-Ereignis
// (Agent-Turns, Approval-Entscheidungen) bekommt einen kettenverknüpften
// Hash. hash(n) = sha256("seq|action|actor|kind|subject|detail|hash(n-1)").
// Manipulation einer Zeile invalidiert alle Nachfolger — Verifikation via
// GET /api/audit/verify.
package store

import (
	"crypto/sha256"
	"fmt"
)

// AuditEvent is one hash-chained, append-only bookkeeping entry.
type AuditEvent struct {
	Seq       int    `json:"seq"`
	Action    string `json:"action"`
	Actor     string `json:"actor"`
	ActorKind string `json:"actorKind"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	PrevHash  string `json:"prevHash"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"createdAt"`
}

// AppendAudit persists one event and chains it onto the current tip.
// Serialized by the single-writer DB handle (MaxOpenConns=1), so the
// MAX(seq)+tip-hash read and the INSERT are race-free.
func (d *DB) AppendAudit(e *AuditEvent) error {
	if e.CreatedAt == "" {
		e.CreatedAt = now()
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	var seq int64
	var prev string
	if err := tx.QueryRow(
		`SELECT COALESCE((SELECT MAX(seq) FROM audit_events), 0),
		        COALESCE((SELECT hash FROM audit_events ORDER BY seq DESC LIMIT 1), '')`,
	).Scan(&seq, &prev); err != nil {
		return err
	}
	e.Seq = int(seq) + 1
	e.PrevHash = prev
	if e.Subject == "" {
		e.Subject = "-"
	}
	block := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s",
		e.Seq, e.Action, e.Actor, e.ActorKind, e.Subject, e.Detail, e.PrevHash)
	e.Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(block)))

	_, err = tx.Exec(`INSERT INTO audit_events
		(seq, action, actor, actor_kind, subject, detail, prev_hash, hash, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.Seq, e.Action, e.Actor, e.ActorKind, e.Subject, e.Detail, e.PrevHash, e.Hash, e.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListAudit returns the newest limit events (newest first).
func (d *DB) ListAudit(limit int) ([]*AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.db.Query(`SELECT seq, action, actor, actor_kind, subject, detail, prev_hash, hash, created_at
		FROM audit_events ORDER BY seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEvent{}
	for rows.Next() {
		e := &AuditEvent{}
		if err := rows.Scan(&e.Seq, &e.Action, &e.Actor, &e.ActorKind, &e.Subject, &e.Detail, &e.PrevHash, &e.Hash, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// VerifyAuditChain recomputes every hash link. Returns ok, the seq where
// the chain first breaks ("" when intact) and the rows verified BEFORE the
// break (gebrochene Zeile zählt nicht — codex P2-2). Leere Kette = ok=false,
// brokenAt="leer" (Genesis fehlt ⇒ Beweis vernichtet oder Seed-Fail —
// niemals still "intakt" melden; codex P2-1-Koroll).
func (d *DB) VerifyAuditChain() (bool, string, int, error) {
	rows, err := d.db.Query(`SELECT seq, action, actor, actor_kind, subject, detail, prev_hash, hash
		FROM audit_events ORDER BY seq`)
	if err != nil {
		return false, "", 0, err
	}
	defer rows.Close()
	prevTip := ""
	n := 0
	for rows.Next() {
		var seq int64
		var e AuditEvent
		if err := rows.Scan(&seq, &e.Action, &e.Actor, &e.ActorKind, &e.Subject, &e.Detail, &e.PrevHash, &e.Hash); err != nil {
			return false, "", n, err
		}
		n++
		block := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s", seq, e.Action, e.Actor, e.ActorKind, e.Subject, e.Detail, e.PrevHash)
		want := fmt.Sprintf("%x", sha256.Sum256([]byte(block)))
		if e.Hash != want || (n > 1 && e.PrevHash != prevTip) {
			v := n - 2
			if v < 0 {
				v = 0
			}
			return false, fmt.Sprintf("seq %d", seq), v, nil
		}
		prevTip = e.Hash
	}
	if n == 0 {
		return false, "leer", 0, rows.Err()
	}
	return true, "", n, rows.Err()
}

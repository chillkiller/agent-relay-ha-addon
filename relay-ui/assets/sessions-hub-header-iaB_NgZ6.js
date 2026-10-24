import{n as e}from"./rolldown-runtime-8BhlS34s.js";import{Eu as t,ju as n}from"./control-ui-core-BncvAZjR.js";import{ir as r,rr as i}from"./control-ui-core-9qYjK5B2.js";import{$ as a,X as o,Y as s}from"./lit-runtime-B8uOr6OX.js";import{m as c,p as l}from"./control-ui-boot-shared-xfDhIVun.js";function u(e){return c({id:`sessions`,active:e.active,tabs:[{value:`sessions`,label:n(`tabs.sessions`)},{value:`worktrees`,label:n(`tabs.worktrees`)}],ariaLabel:n(`sessionsPage.hubTablistLabel`),panelId:`sessions-hub-panel`,onSelect:e.onSelect})}function d(){return(d=e((()=>{t(),l()})))()}function f(e){return a`
    <section
      class="content-header content-header--settings content-header--page hub-page-header sessions-hub-header"
      ${r({toolbarHeader:!0})}
    >
      <div class="hub-page-header__title">
        <div class="page-title">${e.title}</div>
        ${e.subtitle?a`<div class="page-subtitle">${e.subtitle}</div>`:o}
      </div>
      <div class="hub-page-header__tabs">
        ${u({active:e.active,onSelect:e.onSelect})}
      </div>
      <div class="hub-page-header__actions">${e.actions??o}</div>
    </section>
  `}function p(){return(p=e((()=>{s(),i(),d()})))()}export{f as n,p as t};
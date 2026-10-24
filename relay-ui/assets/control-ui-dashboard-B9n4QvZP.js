const __vite__mapDeps=(i,m=__vite__mapDeps,d=(m.f||(m.f=["./board-view-1zQa7RX5.js","./gateway-runtime-Cr51Pi7N.js","./session-snapshot-database-BurLa_ut.js","./control-ui-boot-shared-xfDhIVun.js","./control-ui-boot-shared-Dfs-gZII.js","./control-ui-boot-shared-umGE-sEP.js","./control-ui-boot-shared-Sj2zqjOr.js","./control-ui-boot-shared-em9-jnbz.js","./plugin-contracts-runtime-CNCaYops.js","./control-ui-boot-shared-D4XEBdOG.js","./config-runtime-BecGLziM.js","./markdown-runtime-ORQpioyT.js","./control-ui-boot-shared-DJkUdZmL.js","./control-ui-boot-shared-D3oo_6Fq.js","./control-ui-boot-shared-styles-BaQ2sAfZ.js","./sidebar-update-runtime-DonYLwN5.js","./control-ui-boot-chat-Ck7W_FPO.js","./control-ui-boot-chat-VeeaXyJz.js","./control-ui-boot-chat-DxoNF0Ad.js","./chat-transcript-styles-Dzr3t9kA.js","./control-ui-boot-chat-styles-BRrYkFSc.js","./control-ui-disabled-DfZ2EcL3.js","./near-viewport-observer-WgGck829.js","./widget-sandbox-host-BtiBzdny.js","./mcp-app-confirm-C5ZrOS-h.js","./control-ui-core-yIvVj14Y.css","./control-ui-boot-shared-styles-DKyMAG_6.css","./control-ui-boot-shared-CXHNz0Bh.css","./sidebar-update-runtime-ah7rgQE7.css","./chat-transcript-styles-B_dd5Ueo.css","./control-ui-boot-chat-styles-C_Ls1c0m.css","./control-ui-boot-chat-mLz7NUiY.css","./board-view-DsuslJec.css"])))=>i.map(i=>d[i]);
import{n as e}from"./rolldown-runtime-8BhlS34s.js";import{Cu as t,Eu as n,Tu as r,ju as i}from"./control-ui-core-BncvAZjR.js";import{Fi as a,Ii as o,na as s,ra as c}from"./control-ui-core-9qYjK5B2.js";import{$ as l,X as u,Y as d,ct as f,nt as p,ut as m}from"./lit-runtime-B8uOr6OX.js";import{ii as h,oa as g,sa as _}from"./ui-DuRgK6kL.js";import{Jt as v,Kt as y,Xt as b,qt as x}from"./control-ui-boot-chat-C8hdsB2M.js";var S;function C(){return(C=e((()=>{d(),p(),o(),c(),n(),b(),r(),_(),S=class extends t{constructor(...e){super(...e),this.session=null,this.client=null,this.connected=!1,this.canMutate=!1,this.canGrant=!1,this.presented=!0,this.provider=null,this.expanded=!1,this.activeTabId=``,this.viewError=null,this.viewLoad=null,this.lease=null,this.unsubscribeSnapshot=null,this.expansionInitialized=!1}connectedCallback(){super.connectedCallback(),this.requestUpdate()}updated(){this.viewLoad??=a(`openclaw-board-view`,()=>g(()=>import(`./board-view-1zQa7RX5.js`).then(e=>(e.n(),e.t)),__vite__mapDeps([0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32]),import.meta.url)).catch(e=>{this.viewError=e instanceof Error?e.message:String(e)}),this.synchronizeProvider()}disconnectedCallback(){this.releaseProvider(),super.disconnectedCallback()}synchronizeProvider(){let e=this.session,t=this.client;if(!this.isConnected||!e?.sessionKey.trim()||!t){this.releaseProvider();return}let n=v(e);if(this.lease?.client===t&&this.lease.cacheKey===n){(this.lease.session.sessionKey!==e.sessionKey||this.lease.session.agentId!==e.agentId)&&(this.lease.session={...e},this.requestUpdate()),this.lease.update(t,this.connected,{canPinWidgets:!1,canPinMcpApps:!1,canMutate:this.canMutate,canGrant:this.canGrant});return}this.releaseProvider(),this.expansionInitialized=!1,this.activeTabId=``;let r=y(e,t,this.connected,!1,!1,this.canMutate,this.canGrant);this.lease={...r,client:t,cacheKey:n,session:{...e}},this.provider=r.provider,this.unsubscribeSnapshot=r.provider.snapshot$.subscribe(()=>{this.reconcileSnapshot(r.provider),this.requestUpdate()}),this.reconcileSnapshot(r.provider),this.requestUpdate()}releaseProvider(){this.unsubscribeSnapshot?.(),this.unsubscribeSnapshot=null,this.lease?.release(),this.lease=null,this.provider=null}reconcileSnapshot(e){let t=e.snapshot$.value,n=t.tabs[0]?.tabId??``;t.tabs.some(e=>e.tabId===this.activeTabId)||(this.activeTabId=n),!this.expansionInitialized&&e.hasLoadedSnapshot&&(this.expansionInitialized=!0,this.expanded=x(t))}render(){let e=this.provider,t=e?.snapshot$.value,n=this.lease?.session,r=!!(t&&x(t)),a=e?{appViewGeneration:e.appViewGeneration,applyOps:t=>e.applyOps(t),grant:(t,n)=>e.grant(t,n),selectTab:e=>{this.activeTabId=e},frameLoadFailed:t=>e.refreshWidgetFrame(t),widgetAppView:(t,n)=>e.widgetAppView(t,n),refreshWidgetAppView:(t,n)=>e.refreshWidgetAppView(t,n)}:null;return l`
      <section class="plugin-session-dashboard">
        <button
          type="button"
          class="plugin-session-dashboard__toggle"
          aria-expanded=${this.expanded?`true`:`false`}
          @click=${()=>{this.expansionInitialized=!0,this.expanded=!this.expanded}}
        >
          <span class="plugin-session-dashboard__title">
            ${s.kanban}<span>${i(`pluginUi.dashboardTitle`)}</span>
          </span>
          <span class="plugin-session-dashboard__chevron" aria-hidden="true"
            >${s.arrowDown}</span
          >
        </button>
        <div class="plugin-session-dashboard__body" ?hidden=${!this.expanded}>
          ${this.viewError?l`<p role="alert">${this.viewError}</p>
                  <button
                    type="button"
                    @click=${()=>{this.viewLoad=null,this.viewError=null}}
                  >
                    ${i(`common.retry`)}
                  </button>`:r&&e&&t&&n&&a?l`
                    <openclaw-board-view
                      .active=${this.expanded&&this.presented}
                      .session=${n}
                      .snapshot=${t}
                      .activeTabId=${this.activeTabId}
                      .widgetFrameUrl=${(t,n)=>e.widgetFrameUrl(t,n)}
                      .callbacks=${a}
                      .sessions=${[]}
                      .canMutate=${this.canMutate}
                      .canGrant=${this.canGrant}
                    ></openclaw-board-view>
                  `:l`<p class="plugin-session-dashboard__empty">
                    ${i(`pluginUi.dashboardEmpty`)}
                  </p>`}
        </div>
        ${!this.expanded&&this.expansionInitialized&&!r?l`<p class="plugin-session-dashboard__collapsed-empty">
                ${i(`pluginUi.dashboardEmpty`)}
              </p>`:u}
      </section>
    `}},h([m({attribute:!1})],S.prototype,`session`,void 0),h([m({attribute:!1})],S.prototype,`client`,void 0),h([m({attribute:!1})],S.prototype,`connected`,void 0),h([m({attribute:!1})],S.prototype,`canMutate`,void 0),h([m({attribute:!1})],S.prototype,`canGrant`,void 0),h([m({attribute:!1})],S.prototype,`presented`,void 0),h([f()],S.prototype,`provider`,void 0),h([f()],S.prototype,`expanded`,void 0),h([f()],S.prototype,`activeTabId`,void 0),h([f()],S.prototype,`viewError`,void 0),customElements.get(`openclaw-plugin-session-dashboard`)||customElements.define(`openclaw-plugin-session-dashboard`,S)})))()}C();
import{n as e}from"./rolldown-runtime-8BhlS34s.js";import{Eu as t,Su as n,Tu as r,ju as i}from"./control-ui-core-BncvAZjR.js";import{$t as a,Xt as o,di as s,fi as c,it as l,ot as u}from"./control-ui-core-9qYjK5B2.js";import{$ as d,X as f,Y as p,nt as m,ut as h}from"./lit-runtime-B8uOr6OX.js";import{ii as g}from"./ui-DuRgK6kL.js";import{ea as _,ta as v}from"./control-ui-boot-shared-xfDhIVun.js";import{n as y}from"./browser-panel-CpBLyCyg.js";var b;function x(){return(x=e((()=>{p(),m(),o(),l(),t(),r(),s(),y(),_(),b=class extends n{constructor(...e){super(...e),this.props=null}render(){if(!this.props)return f;let{context:e,target:t,renderEscape:n}=this.props,r=e.gateway.snapshot,o=r.phase===`connected`,s=r.phase===`starting`?i(`common.gatewayStarting`):void 0,l=v(t.tab),p=!!(l&&u(r));return d`
      <openclaw-browser-panel
        embedded
        style=${p?`height: 100dvh;`:`display: none;`}
        .client=${o?r.client:null}
        .available=${p}
        .remoteAvailable=${p}
        .presented=${!0}
        .sessionKey=${t.sessionKey}
        .fixedTab=${l}
        .resourceBasePath=${e.resourceBasePath}
        .authToken=${a({hello:r.hello,settings:{token:e.gateway.connection.token},password:e.gateway.connection.password})}
      ></openclaw-browser-panel>
      ${!o&&r.lastError===null?c(s):f}
      ${!p&&(o||r.lastError)?d`<main class="connect-splash" role="status">
              <div class="stack">
                <span>${i(l?`browser.unavailable`:`focus.unsupported`)}</span>
                ${n(i(`common.back`))}
              </div>
            </main>`:f}
    `}},g([h({attribute:!1})],b.prototype,`props`,void 0),customElements.get(`openclaw-browser-document`)||customElements.define(`openclaw-browser-document`,b)})))()}x();
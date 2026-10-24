import{n as e}from"./rolldown-runtime-8BhlS34s.js";import{Su as t,Tu as n}from"./control-ui-core-BncvAZjR.js";import{$ as r,X as i,Y as a,ct as o,nt as s,ut as c}from"./lit-runtime-B8uOr6OX.js";import{ii as l}from"./ui-DuRgK6kL.js";import{A as u,j as d}from"./control-ui-boot-chat-C8hdsB2M.js";var f;function p(){return(p=e((()=>{a(),s(),d(),n(),f=class extends t{constructor(...e){super(...e),this.routeUrl=null,this.authTokens=[],this.authReady=!1,this.fallback=i,this.undecodableRouteUrl=null,this.loader=new u(this,{cacheNotFound:!0})}render(){return this.loader.withActiveRoutes(()=>this.renderContent())}renderContent(){let e=this.routeUrl,t=e&&this.authReady&&this.undecodableRouteUrl!==e?this.loader.resolve(e,this.authTokens):null;return t?r`<img
      class="channel-avatar"
      src=${t}
      alt=""
      aria-hidden="true"
      decoding="async"
      @error=${()=>{this.undecodableRouteUrl=e}}
    />`:this.fallback}},l([c({attribute:!1})],f.prototype,`routeUrl`,void 0),l([c({attribute:!1})],f.prototype,`authTokens`,void 0),l([c({attribute:!1})],f.prototype,`authReady`,void 0),l([c({attribute:!1})],f.prototype,`fallback`,void 0),l([o()],f.prototype,`undecodableRouteUrl`,void 0),customElements.get(`openclaw-channel-avatar`)||customElements.define(`openclaw-channel-avatar`,f)})))()}p();
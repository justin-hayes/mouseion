var k=globalThis,M=k.ShadowRoot&&(k.ShadyCSS===void 0||k.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,L=Symbol(),Q=new WeakMap,S=class{constructor(e,t,s){if(this._$cssResult$=!0,s!==L)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=e,this.t=t}get styleSheet(){let e=this.o,t=this.t;if(M&&e===void 0){let s=t!==void 0&&t.length===1;s&&(e=Q.get(t)),e===void 0&&((this.o=e=new CSSStyleSheet).replaceSync(this.cssText),s&&Q.set(t,e))}return e}toString(){return this.cssText}},X=n=>new S(typeof n=="string"?n:n+"",void 0,L),D=(n,...e)=>{let t=n.length===1?n[0]:e.reduce((s,r,i)=>s+(o=>{if(o._$cssResult$===!0)return o.cssText;if(typeof o=="number")return o;throw Error("Value passed to 'css' function must be a 'css' function result: "+o+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(r)+n[i+1],n[0]);return new S(t,n,L)},ee=(n,e)=>{if(M)n.adoptedStyleSheets=e.map(t=>t instanceof CSSStyleSheet?t:t.styleSheet);else for(let t of e){let s=document.createElement("style"),r=k.litNonce;r!==void 0&&s.setAttribute("nonce",r),s.textContent=t.cssText,n.appendChild(s)}},j=M?n=>n:n=>n instanceof CSSStyleSheet?(e=>{let t="";for(let s of e.cssRules)t+=s.cssText;return X(t)})(n):n;var{is:ge,defineProperty:$e,getOwnPropertyDescriptor:ye,getOwnPropertyNames:_e,getOwnPropertySymbols:ve,getPrototypeOf:Ae}=Object,R=globalThis,te=R.trustedTypes,Ee=te?te.emptyScript:"",be=R.reactiveElementPolyfillSupport,w=(n,e)=>n,B={toAttribute(n,e){switch(e){case Boolean:n=n?Ee:null;break;case Object:case Array:n=n==null?n:JSON.stringify(n)}return n},fromAttribute(n,e){let t=n;switch(e){case Boolean:t=n!==null;break;case Number:t=n===null?null:Number(n);break;case Object:case Array:try{t=JSON.parse(n)}catch{t=null}}return t}},re=(n,e)=>!ge(n,e),se={attribute:!0,type:String,converter:B,reflect:!1,useDefault:!1,hasChanged:re};Symbol.metadata??=Symbol("metadata"),R.litPropertyMetadata??=new WeakMap;var m=class extends HTMLElement{static addInitializer(e){this._$Ei(),(this.l??=[]).push(e)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(e,t=se){if(t.state&&(t.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(e)&&((t=Object.create(t)).wrapped=!0),this.elementProperties.set(e,t),!t.noAccessor){let s=Symbol(),r=this.getPropertyDescriptor(e,s,t);r!==void 0&&$e(this.prototype,e,r)}}static getPropertyDescriptor(e,t,s){let{get:r,set:i}=ye(this.prototype,e)??{get(){return this[t]},set(o){this[t]=o}};return{get:r,set(o){let h=r?.call(this);i?.call(this,o),this.requestUpdate(e,h,s)},configurable:!0,enumerable:!0}}static getPropertyOptions(e){return this.elementProperties.get(e)??se}static _$Ei(){if(this.hasOwnProperty(w("elementProperties")))return;let e=Ae(this);e.finalize(),e.l!==void 0&&(this.l=[...e.l]),this.elementProperties=new Map(e.elementProperties)}static finalize(){if(this.hasOwnProperty(w("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(w("properties"))){let t=this.properties,s=[..._e(t),...ve(t)];for(let r of s)this.createProperty(r,t[r])}let e=this[Symbol.metadata];if(e!==null){let t=litPropertyMetadata.get(e);if(t!==void 0)for(let[s,r]of t)this.elementProperties.set(s,r)}this._$Eh=new Map;for(let[t,s]of this.elementProperties){let r=this._$Eu(t,s);r!==void 0&&this._$Eh.set(r,t)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(e){let t=[];if(Array.isArray(e)){let s=new Set(e.flat(1/0).reverse());for(let r of s)t.unshift(j(r))}else e!==void 0&&t.push(j(e));return t}static _$Eu(e,t){let s=t.attribute;return s===!1?void 0:typeof s=="string"?s:typeof e=="string"?e.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(e=>this.enableUpdating=e),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(e=>e(this))}addController(e){(this._$EO??=new Set).add(e),this.renderRoot!==void 0&&this.isConnected&&e.hostConnected?.()}removeController(e){this._$EO?.delete(e)}_$E_(){let e=new Map,t=this.constructor.elementProperties;for(let s of t.keys())this.hasOwnProperty(s)&&(e.set(s,this[s]),delete this[s]);e.size>0&&(this._$Ep=e)}createRenderRoot(){let e=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return ee(e,this.constructor.elementStyles),e}connectedCallback(){this.renderRoot??=this.createRenderRoot(),this.enableUpdating(!0),this._$EO?.forEach(e=>e.hostConnected?.())}enableUpdating(e){}disconnectedCallback(){this._$EO?.forEach(e=>e.hostDisconnected?.())}attributeChangedCallback(e,t,s){this._$AK(e,s)}_$ET(e,t){let s=this.constructor.elementProperties.get(e),r=this.constructor._$Eu(e,s);if(r!==void 0&&s.reflect===!0){let i=(s.converter?.toAttribute!==void 0?s.converter:B).toAttribute(t,s.type);this._$Em=e,i==null?this.removeAttribute(r):this.setAttribute(r,i),this._$Em=null}}_$AK(e,t){let s=this.constructor,r=s._$Eh.get(e);if(r!==void 0&&this._$Em!==r){let i=s.getPropertyOptions(r),o=typeof i.converter=="function"?{fromAttribute:i.converter}:i.converter?.fromAttribute!==void 0?i.converter:B;this._$Em=r;let h=o.fromAttribute(t,i.type);this[r]=h??this._$Ej?.get(r)??h,this._$Em=null}}requestUpdate(e,t,s,r=!1,i){if(e!==void 0){let o=this.constructor;if(r===!1&&(i=this[e]),s??=o.getPropertyOptions(e),!((s.hasChanged??re)(i,t)||s.useDefault&&s.reflect&&i===this._$Ej?.get(e)&&!this.hasAttribute(o._$Eu(e,s))))return;this.C(e,t,s)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(e,t,{useDefault:s,reflect:r,wrapped:i},o){s&&!(this._$Ej??=new Map).has(e)&&(this._$Ej.set(e,o??t??this[e]),i!==!0||o!==void 0)||(this._$AL.has(e)||(this.hasUpdated||s||(t=void 0),this._$AL.set(e,t)),r===!0&&this._$Em!==e&&(this._$Eq??=new Set).add(e))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(t){Promise.reject(t)}let e=this.scheduleUpdate();return e!=null&&await e,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??=this.createRenderRoot(),this._$Ep){for(let[r,i]of this._$Ep)this[r]=i;this._$Ep=void 0}let s=this.constructor.elementProperties;if(s.size>0)for(let[r,i]of s){let{wrapped:o}=i,h=this[r];o!==!0||this._$AL.has(r)||h===void 0||this.C(r,void 0,i,h)}}let e=!1,t=this._$AL;try{e=this.shouldUpdate(t),e?(this.willUpdate(t),this._$EO?.forEach(s=>s.hostUpdate?.()),this.update(t)):this._$EM()}catch(s){throw e=!1,this._$EM(),s}e&&this._$AE(t)}willUpdate(e){}_$AE(e){this._$EO?.forEach(t=>t.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(e)),this.updated(e)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(e){return!0}update(e){this._$Eq&&=this._$Eq.forEach(t=>this._$ET(t,this[t])),this._$EM()}updated(e){}firstUpdated(e){}};m.elementStyles=[],m.shadowRootOptions={mode:"open"},m[w("elementProperties")]=new Map,m[w("finalized")]=new Map,be?.({ReactiveElement:m}),(R.reactiveElementVersions??=[]).push("2.1.2");var W=globalThis,ne=n=>n,H=W.trustedTypes,ie=H?H.createPolicy("lit-html",{createHTML:n=>n}):void 0,de="$lit$",g=`lit$${Math.random().toFixed(9).slice(2)}$`,pe="?"+g,Se=`<${pe}>`,v=document,C=()=>v.createComment(""),P=n=>n===null||typeof n!="object"&&typeof n!="function",J=Array.isArray,we=n=>J(n)||typeof n?.[Symbol.iterator]=="function",I=`[\x20\t\n\f\r]`,x=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,oe=/-->/g,ae=/>/g,y=RegExp(`>|${I}(?:([^\\s"'>=/]+)(${I}*=${I}*(?:[^\x20\t\n\f\r"'\`<>=]|("|')|))|$)`,"g"),ce=/'/g,le=/"/g,ue=/^(?:script|style|textarea|title)$/i,Y=n=>(e,...t)=>({_$litType$:n,strings:e,values:t}),N=Y(1),Re=Y(2),He=Y(3),A=Symbol.for("lit-noChange"),l=Symbol.for("lit-nothing"),he=new WeakMap,_=v.createTreeWalker(v,129);function me(n,e){if(!J(n)||!n.hasOwnProperty("raw"))throw Error("invalid template strings array");return ie!==void 0?ie.createHTML(e):e}var xe=(n,e)=>{let t=n.length-1,s=[],r,i=e===2?"<svg>":e===3?"<math>":"",o=x;for(let h=0;h<t;h++){let a=n[h],d,p,c=-1,u=0;for(;u<a.length&&(o.lastIndex=u,p=o.exec(a),p!==null);)u=o.lastIndex,o===x?p[1]==="!--"?o=oe:p[1]!==void 0?o=ae:p[2]!==void 0?(ue.test(p[2])&&(r=RegExp("</"+p[2],"g")),o=y):p[3]!==void 0&&(o=y):o===y?p[0]===">"?(o=r??x,c=-1):p[1]===void 0?c=-2:(c=o.lastIndex-p[2].length,d=p[1],o=p[3]===void 0?y:p[3]==='"'?le:ce):o===le||o===ce?o=y:o===oe||o===ae?o=x:(o=y,r=void 0);let f=o===y&&n[h+1].startsWith("/>")?" ":"";i+=o===x?a+Se:c>=0?(s.push(d),a.slice(0,c)+de+a.slice(c)+g+f):a+g+(c===-2?h:f)}return[me(n,i+(n[t]||"<?>")+(e===2?"</svg>":e===3?"</math>":"")),s]},U=class n{constructor({strings:e,_$litType$:t},s){let r;this.parts=[];let i=0,o=0,h=e.length-1,a=this.parts,[d,p]=xe(e,t);if(this.el=n.createElement(d,s),_.currentNode=this.el.content,t===2||t===3){let c=this.el.content.firstChild;c.replaceWith(...c.childNodes)}for(;(r=_.nextNode())!==null&&a.length<h;){if(r.nodeType===1){if(r.hasAttributes())for(let c of r.getAttributeNames())if(c.endsWith(de)){let u=p[o++],f=r.getAttribute(c).split(g),O=/([.?@])?(.*)/.exec(u);a.push({type:1,index:i,name:O[2],strings:f,ctor:O[1]==="."?z:O[1]==="?"?q:O[1]==="@"?K:b}),r.removeAttribute(c)}else c.startsWith(g)&&(a.push({type:6,index:i}),r.removeAttribute(c));if(ue.test(r.tagName)){let c=r.textContent.split(g),u=c.length-1;if(u>0){r.textContent=H?H.emptyScript:"";for(let f=0;f<u;f++)r.append(c[f],C()),_.nextNode(),a.push({type:2,index:++i});r.append(c[u],C())}}}else if(r.nodeType===8)if(r.data===pe)a.push({type:2,index:i});else{let c=-1;for(;(c=r.data.indexOf(g,c+1))!==-1;)a.push({type:7,index:i}),c+=g.length-1}i++}}static createElement(e,t){let s=v.createElement("template");return s.innerHTML=e,s}};function E(n,e,t=n,s){if(e===A)return e;let r=s!==void 0?t._$Co?.[s]:t._$Cl,i=P(e)?void 0:e._$litDirective$;return r?.constructor!==i&&(r?._$AO?.(!1),i===void 0?r=void 0:(r=new i(n),r._$AT(n,t,s)),s!==void 0?(t._$Co??=[])[s]=r:t._$Cl=r),r!==void 0&&(e=E(n,r._$AS(n,e.values),r,s)),e}var F=class{constructor(e,t){this._$AV=[],this._$AN=void 0,this._$AD=e,this._$AM=t}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(e){let{el:{content:t},parts:s}=this._$AD,r=(e?.creationScope??v).importNode(t,!0);_.currentNode=r;let i=_.nextNode(),o=0,h=0,a=s[0];for(;a!==void 0;){if(o===a.index){let d;a.type===2?d=new T(i,i.nextSibling,this,e):a.type===1?d=new a.ctor(i,a.name,a.strings,this,e):a.type===6&&(d=new V(i,this,e)),this._$AV.push(d),a=s[++h]}o!==a?.index&&(i=_.nextNode(),o++)}return _.currentNode=v,r}p(e){let t=0;for(let s of this._$AV)s!==void 0&&(s.strings!==void 0?(s._$AI(e,s,t),t+=s.strings.length-2):s._$AI(e[t])),t++}},T=class n{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(e,t,s,r){this.type=2,this._$AH=l,this._$AN=void 0,this._$AA=e,this._$AB=t,this._$AM=s,this.options=r,this._$Cv=r?.isConnected??!0}get parentNode(){let e=this._$AA.parentNode,t=this._$AM;return t!==void 0&&e?.nodeType===11&&(e=t.parentNode),e}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(e,t=this){e=E(this,e,t),P(e)?e===l||e==null||e===""?(this._$AH!==l&&this._$AR(),this._$AH=l):e!==this._$AH&&e!==A&&this._(e):e._$litType$!==void 0?this.$(e):e.nodeType!==void 0?this.T(e):we(e)?this.k(e):this._(e)}O(e){return this._$AA.parentNode.insertBefore(e,this._$AB)}T(e){this._$AH!==e&&(this._$AR(),this._$AH=this.O(e))}_(e){this._$AH!==l&&P(this._$AH)?this._$AA.nextSibling.data=e:this.T(v.createTextNode(e)),this._$AH=e}$(e){let{values:t,_$litType$:s}=e,r=typeof s=="number"?this._$AC(e):(s.el===void 0&&(s.el=U.createElement(me(s.h,s.h[0]),this.options)),s);if(this._$AH?._$AD===r)this._$AH.p(t);else{let i=new F(r,this),o=i.u(this.options);i.p(t),this.T(o),this._$AH=i}}_$AC(e){let t=he.get(e.strings);return t===void 0&&he.set(e.strings,t=new U(e)),t}k(e){J(this._$AH)||(this._$AH=[],this._$AR());let t=this._$AH,s,r=0;for(let i of e)r===t.length?t.push(s=new n(this.O(C()),this.O(C()),this,this.options)):s=t[r],s._$AI(i),r++;r<t.length&&(this._$AR(s&&s._$AB.nextSibling,r),t.length=r)}_$AR(e=this._$AA.nextSibling,t){for(this._$AP?.(!1,!0,t);e!==this._$AB;){let s=ne(e).nextSibling;ne(e).remove(),e=s}}setConnected(e){this._$AM===void 0&&(this._$Cv=e,this._$AP?.(e))}},b=class{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(e,t,s,r,i){this.type=1,this._$AH=l,this._$AN=void 0,this.element=e,this.name=t,this._$AM=r,this.options=i,s.length>2||s[0]!==""||s[1]!==""?(this._$AH=Array(s.length-1).fill(new String),this.strings=s):this._$AH=l}_$AI(e,t=this,s,r){let i=this.strings,o=!1;if(i===void 0)e=E(this,e,t,0),o=!P(e)||e!==this._$AH&&e!==A,o&&(this._$AH=e);else{let h=e,a,d;for(e=i[0],a=0;a<i.length-1;a++)d=E(this,h[s+a],t,a),d===A&&(d=this._$AH[a]),o||=!P(d)||d!==this._$AH[a],d===l?e=l:e!==l&&(e+=(d??"")+i[a+1]),this._$AH[a]=d}o&&!r&&this.j(e)}j(e){e===l?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,e??"")}},z=class extends b{constructor(){super(...arguments),this.type=3}j(e){this.element[this.name]=e===l?void 0:e}},q=class extends b{constructor(){super(...arguments),this.type=4}j(e){this.element.toggleAttribute(this.name,!!e&&e!==l)}},K=class extends b{constructor(e,t,s,r,i){super(e,t,s,r,i),this.type=5}_$AI(e,t=this){if((e=E(this,e,t,0)??l)===A)return;let s=this._$AH,r=e===l&&s!==l||e.capture!==s.capture||e.once!==s.once||e.passive!==s.passive,i=e!==l&&(s===l||r);r&&this.element.removeEventListener(this.name,this,s),i&&this.element.addEventListener(this.name,this,e),this._$AH=e}handleEvent(e){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,e):this._$AH.handleEvent(e)}},V=class{constructor(e,t,s){this.element=e,this.type=6,this._$AN=void 0,this._$AM=t,this.options=s}get _$AU(){return this._$AM._$AU}_$AI(e){E(this,e)}};var Ce=W.litHtmlPolyfillSupport;Ce?.(U,T),(W.litHtmlVersions??=[]).push("3.3.3");var fe=(n,e,t)=>{let s=t?.renderBefore??e,r=s._$litPart$;if(r===void 0){let i=t?.renderBefore??null;s._$litPart$=r=new T(e.insertBefore(C(),i),i,void 0,t??{})}return r._$AI(n),r};var Z=globalThis,$=class extends m{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){let e=super.createRenderRoot();return this.renderOptions.renderBefore??=e.firstChild,e}update(e){let t=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(e),this._$Do=fe(t,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return A}};$._$litElement$=!0,$.finalized=!0,Z.litElementHydrateSupport?.({LitElement:$});var Pe=Z.litElementPolyfillSupport;Pe?.({LitElement:$});(Z.litElementVersions??=[]).push("4.2.2");function Ue(n){if(!n||typeof n!="object")return!1;let e=n;return typeof e.id=="string"&&/^occurrence-[\w-]+-\d+-\d+$/.test(e.id)&&typeof e.bookTitle=="string"&&e.bookTitle.length>0&&typeof e.left=="string"&&typeof e.surface=="string"&&e.surface.length>0&&typeof e.right=="string"&&typeof e.sentence=="string"&&Number.isInteger(e.targetStart)&&Number.isInteger(e.targetEnd)&&e.targetStart>=0&&e.targetEnd>e.targetStart&&e.targetEnd<=e.sentence.length&&e.sentence.slice(e.targetStart,e.targetEnd)===e.surface&&typeof e.studyUrl=="string"&&e.studyUrl.startsWith("/vocabulary/concordance/sentence?")}var G=class extends ${static styles=D`
    :host {
      display: block;
      color: var(--mouseion-color-text, inherit);
      font-family: var(--mouseion-font-application, sans-serif);
    }
    .concordance-results {
      list-style: none;
      padding-inline-start: 0;
      margin-block: var(--mouseion-space-3, 0.75rem);
    }
    .concordance-result {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: stretch;
      gap: var(--mouseion-space-2, 0.5rem);
      border-bottom: 1px solid var(--mouseion-color-border, currentColor);
    }
    .concordance-row {
      min-width: 0;
      margin: 0;
      border: 0;
      border-radius: 0;
      padding-block: var(--mouseion-space-2, 0.5rem);
    }
    .concordance-row summary {
      display: grid;
      grid-template-columns: minmax(9rem, 0.8fr) minmax(0, 1fr) max-content minmax(0, 1fr);
      align-items: baseline;
      gap: var(--mouseion-space-2, 0.5rem);
      line-height: 1.5;
      max-width: 100%;
      cursor: pointer;
    }
    .concordance-book-title {
      min-width: 0;
      font-family: var(--mouseion-font-reading, serif);
      font-weight: 600;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .concordance-before,
    .concordance-after {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .concordance-surface,
    .concordance-observed-target {
      color: var(--mouseion-color-accent, currentColor);
      white-space: nowrap;
    }
    .concordance-study-link {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: var(--mouseion-space-1, 0.25rem);
      min-width: 2.75rem;
      padding-inline: var(--mouseion-space-2, 0.5rem);
      color: var(--mouseion-color-accent, currentColor);
      text-align: center;
    }
    .concordance-context {
      max-width: var(--mouseion-width-reading, 42rem);
      padding: var(--mouseion-space-2, 0.5rem) var(--mouseion-space-3, 0.75rem) 0;
    }
    .concordance-context p {
      overflow-wrap: anywhere;
      margin-block: 0 var(--mouseion-space-2, 0.5rem);
      font-family: var(--mouseion-font-reading, serif);
      line-height: 1.7;
    }
    :focus-visible {
      outline: 2px solid var(--mouseion-color-focus, currentColor);
      outline-offset: 2px;
    }
    @media (max-width: 40rem) {
      .concordance-result { grid-template-columns: minmax(0, 1fr) auto; }
      .concordance-row summary {
        grid-template-columns: minmax(0, 1fr) max-content;
        gap: var(--mouseion-space-1, 0.25rem) var(--mouseion-space-2, 0.5rem);
      }
      .concordance-book-title {
        grid-column: 1 / -1;
        white-space: normal;
        overflow-wrap: anywhere;
      }
      .concordance-before { grid-column: 1; white-space: normal; }
      .concordance-surface { grid-column: 2; grid-row: 2; }
      .concordance-after { grid-column: 1 / -1; white-space: normal; }
      .concordance-study-link {
        align-self: start;
        min-width: 0;
        padding-inline: var(--mouseion-space-1, 0.25rem);
      }
      .concordance-study-label { display: inline; }
    }
  `;occurrences=null;connectedCallback(){super.connectedCallback(),window.addEventListener("hashchange",this.restoreFragmentFocus),window.addEventListener("popstate",this.restoreFragmentFocus);try{let e=this.parentElement,t=document.getElementById("concordance-native-results");if(!e||!t)return;let s=JSON.parse(e.getAttribute("data-concordance-data")??"");if(!s||typeof s!="object"||!Array.isArray(s.occurrences)||!s.occurrences.every(Ue)||s.occurrences.length===0)return;this.occurrences=s.occurrences,this.requestUpdate(),this.updateComplete.then(()=>{!this.isConnected||!this.occurrences||(t.remove(),e.hidden=!1,this.restoreFragmentFocus())})}catch{}}disconnectedCallback(){window.removeEventListener("hashchange",this.restoreFragmentFocus),window.removeEventListener("popstate",this.restoreFragmentFocus),super.disconnectedCallback()}restoreFragmentFocus=()=>{if(!this.isConnected||!this.occurrences||!location.hash)return;let e=location.hash.slice(1);(this.shadowRoot?.getElementById(e)??document.getElementById("concordance-summary"))?.focus({preventScroll:!0})};render(){return this.occurrences?N`<ol class="concordance-results">
      ${this.occurrences.map(e=>N`<li class="concordance-result">
        <details class="concordance-row" id=${e.id} tabindex="-1" @keydown=${this.onRowKeyDown}>
          <summary aria-label=${`Occurrence of ${e.surface} in ${e.bookTitle}`} @click=${this.onRowClick}>
            <span class="concordance-book-title">${e.bookTitle}</span>
            <span class="concordance-before">${e.left}</span>
            <strong class="concordance-surface">${e.surface}</strong>
            <span class="concordance-after">${e.right}</span>
          </summary>
          <div class="concordance-context"><p class="reading-text">${this.sentence(e)}</p></div>
        </details>
        <a class="concordance-study-link" aria-label="Study this sentence and its syntax" href=${e.studyUrl}>
          <span aria-hidden="true">↗</span><span class="concordance-study-label">Study</span>
        </a>
      </li>`)}
    </ol>`:l}sentence(e){return N`${e.sentence.slice(0,e.targetStart)}<strong class="concordance-observed-target">${e.surface}</strong>${e.sentence.slice(e.targetEnd)}`}onRowClick=e=>{let t=e.currentTarget;if(!(t instanceof HTMLElement)||t.tagName!=="SUMMARY")return;let s=t.closest("details.concordance-row");!s||s.open||this.shadowRoot?.querySelectorAll("details.concordance-row[open]").forEach(r=>{r!==s&&(r.open=!1)})};onRowKeyDown=e=>{if(e.altKey||e.ctrlKey||e.metaKey||e.shiftKey)return;let t=e.target;if(!(t instanceof HTMLElement)||t.tagName!=="SUMMARY"||!t.matches(":focus"))return;let s=Array.from(this.shadowRoot?.querySelectorAll("details.concordance-row")??[]),r=t.closest("details.concordance-row"),i=r?s.indexOf(r):-1;i<0||(e.key==="ArrowDown"&&i<s.length-1?(e.preventDefault(),s[i+1].querySelector("summary")?.focus()):e.key==="ArrowUp"&&i>0?(e.preventDefault(),s[i-1].querySelector("summary")?.focus()):e.key==="Escape"&&r?.open&&(e.preventDefault(),r.open=!1,t.focus()))}};customElements.get("mouseion-concordance")||customElements.define("mouseion-concordance",G);
/*! Bundled license information:

@lit/reactive-element/css-tag.js:
  (**
   * @license
   * Copyright 2019 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)

@lit/reactive-element/reactive-element.js:
lit-html/lit-html.js:
lit-element/lit-element.js:
  (**
   * @license
   * Copyright 2017 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)

lit-html/is-server.js:
  (**
   * @license
   * Copyright 2022 Google LLC
   * SPDX-License-Identifier: BSD-3-Clause
   *)
*/

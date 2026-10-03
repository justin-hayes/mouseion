var k=globalThis,R=k.ShadowRoot&&(k.ShadyCSS===void 0||k.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,L=Symbol(),Q=new WeakMap,S=class{constructor(e,t,s){if(this._$cssResult$=!0,s!==L)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=e,this.t=t}get styleSheet(){let e=this.o,t=this.t;if(R&&e===void 0){let s=t!==void 0&&t.length===1;s&&(e=Q.get(t)),e===void 0&&((this.o=e=new CSSStyleSheet).replaceSync(this.cssText),s&&Q.set(t,e))}return e}toString(){return this.cssText}},X=r=>new S(typeof r=="string"?r:r+"",void 0,L),D=(r,...e)=>{let t=r.length===1?r[0]:e.reduce((s,i,n)=>s+(o=>{if(o._$cssResult$===!0)return o.cssText;if(typeof o=="number")return o;throw Error("Value passed to 'css' function must be a 'css' function result: "+o+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(i)+r[n+1],r[0]);return new S(t,r,L)},ee=(r,e)=>{if(R)r.adoptedStyleSheets=e.map(t=>t instanceof CSSStyleSheet?t:t.styleSheet);else for(let t of e){let s=document.createElement("style"),i=k.litNonce;i!==void 0&&s.setAttribute("nonce",i),s.textContent=t.cssText,r.appendChild(s)}},j=R?r=>r:r=>r instanceof CSSStyleSheet?(e=>{let t="";for(let s of e.cssRules)t+=s.cssText;return X(t)})(r):r;var{is:ge,defineProperty:$e,getOwnPropertyDescriptor:_e,getOwnPropertyNames:ye,getOwnPropertySymbols:ve,getPrototypeOf:Ae}=Object,M=globalThis,te=M.trustedTypes,be=te?te.emptyScript:"",Ee=M.reactiveElementPolyfillSupport,w=(r,e)=>r,B={toAttribute(r,e){switch(e){case Boolean:r=r?be:null;break;case Object:case Array:r=r==null?r:JSON.stringify(r)}return r},fromAttribute(r,e){let t=r;switch(e){case Boolean:t=r!==null;break;case Number:t=r===null?null:Number(r);break;case Object:case Array:try{t=JSON.parse(r)}catch{t=null}}return t}},ie=(r,e)=>!ge(r,e),se={attribute:!0,type:String,converter:B,reflect:!1,useDefault:!1,hasChanged:ie};Symbol.metadata??=Symbol("metadata"),M.litPropertyMetadata??=new WeakMap;var m=class extends HTMLElement{static addInitializer(e){this._$Ei(),(this.l??=[]).push(e)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(e,t=se){if(t.state&&(t.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(e)&&((t=Object.create(t)).wrapped=!0),this.elementProperties.set(e,t),!t.noAccessor){let s=Symbol(),i=this.getPropertyDescriptor(e,s,t);i!==void 0&&$e(this.prototype,e,i)}}static getPropertyDescriptor(e,t,s){let{get:i,set:n}=_e(this.prototype,e)??{get(){return this[t]},set(o){this[t]=o}};return{get:i,set(o){let h=i?.call(this);n?.call(this,o),this.requestUpdate(e,h,s)},configurable:!0,enumerable:!0}}static getPropertyOptions(e){return this.elementProperties.get(e)??se}static _$Ei(){if(this.hasOwnProperty(w("elementProperties")))return;let e=Ae(this);e.finalize(),e.l!==void 0&&(this.l=[...e.l]),this.elementProperties=new Map(e.elementProperties)}static finalize(){if(this.hasOwnProperty(w("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(w("properties"))){let t=this.properties,s=[...ye(t),...ve(t)];for(let i of s)this.createProperty(i,t[i])}let e=this[Symbol.metadata];if(e!==null){let t=litPropertyMetadata.get(e);if(t!==void 0)for(let[s,i]of t)this.elementProperties.set(s,i)}this._$Eh=new Map;for(let[t,s]of this.elementProperties){let i=this._$Eu(t,s);i!==void 0&&this._$Eh.set(i,t)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(e){let t=[];if(Array.isArray(e)){let s=new Set(e.flat(1/0).reverse());for(let i of s)t.unshift(j(i))}else e!==void 0&&t.push(j(e));return t}static _$Eu(e,t){let s=t.attribute;return s===!1?void 0:typeof s=="string"?s:typeof e=="string"?e.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(e=>this.enableUpdating=e),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(e=>e(this))}addController(e){(this._$EO??=new Set).add(e),this.renderRoot!==void 0&&this.isConnected&&e.hostConnected?.()}removeController(e){this._$EO?.delete(e)}_$E_(){let e=new Map,t=this.constructor.elementProperties;for(let s of t.keys())this.hasOwnProperty(s)&&(e.set(s,this[s]),delete this[s]);e.size>0&&(this._$Ep=e)}createRenderRoot(){let e=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return ee(e,this.constructor.elementStyles),e}connectedCallback(){this.renderRoot??=this.createRenderRoot(),this.enableUpdating(!0),this._$EO?.forEach(e=>e.hostConnected?.())}enableUpdating(e){}disconnectedCallback(){this._$EO?.forEach(e=>e.hostDisconnected?.())}attributeChangedCallback(e,t,s){this._$AK(e,s)}_$ET(e,t){let s=this.constructor.elementProperties.get(e),i=this.constructor._$Eu(e,s);if(i!==void 0&&s.reflect===!0){let n=(s.converter?.toAttribute!==void 0?s.converter:B).toAttribute(t,s.type);this._$Em=e,n==null?this.removeAttribute(i):this.setAttribute(i,n),this._$Em=null}}_$AK(e,t){let s=this.constructor,i=s._$Eh.get(e);if(i!==void 0&&this._$Em!==i){let n=s.getPropertyOptions(i),o=typeof n.converter=="function"?{fromAttribute:n.converter}:n.converter?.fromAttribute!==void 0?n.converter:B;this._$Em=i;let h=o.fromAttribute(t,n.type);this[i]=h??this._$Ej?.get(i)??h,this._$Em=null}}requestUpdate(e,t,s,i=!1,n){if(e!==void 0){let o=this.constructor;if(i===!1&&(n=this[e]),s??=o.getPropertyOptions(e),!((s.hasChanged??ie)(n,t)||s.useDefault&&s.reflect&&n===this._$Ej?.get(e)&&!this.hasAttribute(o._$Eu(e,s))))return;this.C(e,t,s)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(e,t,{useDefault:s,reflect:i,wrapped:n},o){s&&!(this._$Ej??=new Map).has(e)&&(this._$Ej.set(e,o??t??this[e]),n!==!0||o!==void 0)||(this._$AL.has(e)||(this.hasUpdated||s||(t=void 0),this._$AL.set(e,t)),i===!0&&this._$Em!==e&&(this._$Eq??=new Set).add(e))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(t){Promise.reject(t)}let e=this.scheduleUpdate();return e!=null&&await e,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??=this.createRenderRoot(),this._$Ep){for(let[i,n]of this._$Ep)this[i]=n;this._$Ep=void 0}let s=this.constructor.elementProperties;if(s.size>0)for(let[i,n]of s){let{wrapped:o}=n,h=this[i];o!==!0||this._$AL.has(i)||h===void 0||this.C(i,void 0,n,h)}}let e=!1,t=this._$AL;try{e=this.shouldUpdate(t),e?(this.willUpdate(t),this._$EO?.forEach(s=>s.hostUpdate?.()),this.update(t)):this._$EM()}catch(s){throw e=!1,this._$EM(),s}e&&this._$AE(t)}willUpdate(e){}_$AE(e){this._$EO?.forEach(t=>t.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(e)),this.updated(e)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(e){return!0}update(e){this._$Eq&&=this._$Eq.forEach(t=>this._$ET(t,this[t])),this._$EM()}updated(e){}firstUpdated(e){}};m.elementStyles=[],m.shadowRootOptions={mode:"open"},m[w("elementProperties")]=new Map,m[w("finalized")]=new Map,Ee?.({ReactiveElement:m}),(M.reactiveElementVersions??=[]).push("2.1.2");var K=globalThis,re=r=>r,H=K.trustedTypes,ne=H?H.createPolicy("lit-html",{createHTML:r=>r}):void 0,de="$lit$",g=`lit$${Math.random().toFixed(9).slice(2)}$`,pe="?"+g,Se=`<${pe}>`,v=document,C=()=>v.createComment(""),P=r=>r===null||typeof r!="object"&&typeof r!="function",J=Array.isArray,we=r=>J(r)||typeof r?.[Symbol.iterator]=="function",I=`[\x20\t\n\f\r]`,x=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,oe=/-->/g,ae=/>/g,_=RegExp(`>|${I}(?:([^\\s"'>=/]+)(${I}*=${I}*(?:[^\x20\t\n\f\r"'\`<>=]|("|')|))|$)`,"g"),ce=/'/g,le=/"/g,ue=/^(?:script|style|textarea|title)$/i,Y=r=>(e,...t)=>({_$litType$:r,strings:e,values:t}),N=Y(1),Me=Y(2),He=Y(3),A=Symbol.for("lit-noChange"),l=Symbol.for("lit-nothing"),he=new WeakMap,y=v.createTreeWalker(v,129);function me(r,e){if(!J(r)||!r.hasOwnProperty("raw"))throw Error("invalid template strings array");return ne!==void 0?ne.createHTML(e):e}var xe=(r,e)=>{let t=r.length-1,s=[],i,n=e===2?"<svg>":e===3?"<math>":"",o=x;for(let h=0;h<t;h++){let a=r[h],d,p,c=-1,u=0;for(;u<a.length&&(o.lastIndex=u,p=o.exec(a),p!==null);)u=o.lastIndex,o===x?p[1]==="!--"?o=oe:p[1]!==void 0?o=ae:p[2]!==void 0?(ue.test(p[2])&&(i=RegExp("</"+p[2],"g")),o=_):p[3]!==void 0&&(o=_):o===_?p[0]===">"?(o=i??x,c=-1):p[1]===void 0?c=-2:(c=o.lastIndex-p[2].length,d=p[1],o=p[3]===void 0?_:p[3]==='"'?le:ce):o===le||o===ce?o=_:o===oe||o===ae?o=x:(o=_,i=void 0);let f=o===_&&r[h+1].startsWith("/>")?" ":"";n+=o===x?a+Se:c>=0?(s.push(d),a.slice(0,c)+de+a.slice(c)+g+f):a+g+(c===-2?h:f)}return[me(r,n+(r[t]||"<?>")+(e===2?"</svg>":e===3?"</math>":"")),s]},U=class r{constructor({strings:e,_$litType$:t},s){let i;this.parts=[];let n=0,o=0,h=e.length-1,a=this.parts,[d,p]=xe(e,t);if(this.el=r.createElement(d,s),y.currentNode=this.el.content,t===2||t===3){let c=this.el.content.firstChild;c.replaceWith(...c.childNodes)}for(;(i=y.nextNode())!==null&&a.length<h;){if(i.nodeType===1){if(i.hasAttributes())for(let c of i.getAttributeNames())if(c.endsWith(de)){let u=p[o++],f=i.getAttribute(c).split(g),T=/([.?@])?(.*)/.exec(u);a.push({type:1,index:n,name:T[2],strings:f,ctor:T[1]==="."?z:T[1]==="?"?q:T[1]==="@"?V:E}),i.removeAttribute(c)}else c.startsWith(g)&&(a.push({type:6,index:n}),i.removeAttribute(c));if(ue.test(i.tagName)){let c=i.textContent.split(g),u=c.length-1;if(u>0){i.textContent=H?H.emptyScript:"";for(let f=0;f<u;f++)i.append(c[f],C()),y.nextNode(),a.push({type:2,index:++n});i.append(c[u],C())}}}else if(i.nodeType===8)if(i.data===pe)a.push({type:2,index:n});else{let c=-1;for(;(c=i.data.indexOf(g,c+1))!==-1;)a.push({type:7,index:n}),c+=g.length-1}n++}}static createElement(e,t){let s=v.createElement("template");return s.innerHTML=e,s}};function b(r,e,t=r,s){if(e===A)return e;let i=s!==void 0?t._$Co?.[s]:t._$Cl,n=P(e)?void 0:e._$litDirective$;return i?.constructor!==n&&(i?._$AO?.(!1),n===void 0?i=void 0:(i=new n(r),i._$AT(r,t,s)),s!==void 0?(t._$Co??=[])[s]=i:t._$Cl=i),i!==void 0&&(e=b(r,i._$AS(r,e.values),i,s)),e}var F=class{constructor(e,t){this._$AV=[],this._$AN=void 0,this._$AD=e,this._$AM=t}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(e){let{el:{content:t},parts:s}=this._$AD,i=(e?.creationScope??v).importNode(t,!0);y.currentNode=i;let n=y.nextNode(),o=0,h=0,a=s[0];for(;a!==void 0;){if(o===a.index){let d;a.type===2?d=new O(n,n.nextSibling,this,e):a.type===1?d=new a.ctor(n,a.name,a.strings,this,e):a.type===6&&(d=new W(n,this,e)),this._$AV.push(d),a=s[++h]}o!==a?.index&&(n=y.nextNode(),o++)}return y.currentNode=v,i}p(e){let t=0;for(let s of this._$AV)s!==void 0&&(s.strings!==void 0?(s._$AI(e,s,t),t+=s.strings.length-2):s._$AI(e[t])),t++}},O=class r{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(e,t,s,i){this.type=2,this._$AH=l,this._$AN=void 0,this._$AA=e,this._$AB=t,this._$AM=s,this.options=i,this._$Cv=i?.isConnected??!0}get parentNode(){let e=this._$AA.parentNode,t=this._$AM;return t!==void 0&&e?.nodeType===11&&(e=t.parentNode),e}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(e,t=this){e=b(this,e,t),P(e)?e===l||e==null||e===""?(this._$AH!==l&&this._$AR(),this._$AH=l):e!==this._$AH&&e!==A&&this._(e):e._$litType$!==void 0?this.$(e):e.nodeType!==void 0?this.T(e):we(e)?this.k(e):this._(e)}O(e){return this._$AA.parentNode.insertBefore(e,this._$AB)}T(e){this._$AH!==e&&(this._$AR(),this._$AH=this.O(e))}_(e){this._$AH!==l&&P(this._$AH)?this._$AA.nextSibling.data=e:this.T(v.createTextNode(e)),this._$AH=e}$(e){let{values:t,_$litType$:s}=e,i=typeof s=="number"?this._$AC(e):(s.el===void 0&&(s.el=U.createElement(me(s.h,s.h[0]),this.options)),s);if(this._$AH?._$AD===i)this._$AH.p(t);else{let n=new F(i,this),o=n.u(this.options);n.p(t),this.T(o),this._$AH=n}}_$AC(e){let t=he.get(e.strings);return t===void 0&&he.set(e.strings,t=new U(e)),t}k(e){J(this._$AH)||(this._$AH=[],this._$AR());let t=this._$AH,s,i=0;for(let n of e)i===t.length?t.push(s=new r(this.O(C()),this.O(C()),this,this.options)):s=t[i],s._$AI(n),i++;i<t.length&&(this._$AR(s&&s._$AB.nextSibling,i),t.length=i)}_$AR(e=this._$AA.nextSibling,t){for(this._$AP?.(!1,!0,t);e!==this._$AB;){let s=re(e).nextSibling;re(e).remove(),e=s}}setConnected(e){this._$AM===void 0&&(this._$Cv=e,this._$AP?.(e))}},E=class{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(e,t,s,i,n){this.type=1,this._$AH=l,this._$AN=void 0,this.element=e,this.name=t,this._$AM=i,this.options=n,s.length>2||s[0]!==""||s[1]!==""?(this._$AH=Array(s.length-1).fill(new String),this.strings=s):this._$AH=l}_$AI(e,t=this,s,i){let n=this.strings,o=!1;if(n===void 0)e=b(this,e,t,0),o=!P(e)||e!==this._$AH&&e!==A,o&&(this._$AH=e);else{let h=e,a,d;for(e=n[0],a=0;a<n.length-1;a++)d=b(this,h[s+a],t,a),d===A&&(d=this._$AH[a]),o||=!P(d)||d!==this._$AH[a],d===l?e=l:e!==l&&(e+=(d??"")+n[a+1]),this._$AH[a]=d}o&&!i&&this.j(e)}j(e){e===l?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,e??"")}},z=class extends E{constructor(){super(...arguments),this.type=3}j(e){this.element[this.name]=e===l?void 0:e}},q=class extends E{constructor(){super(...arguments),this.type=4}j(e){this.element.toggleAttribute(this.name,!!e&&e!==l)}},V=class extends E{constructor(e,t,s,i,n){super(e,t,s,i,n),this.type=5}_$AI(e,t=this){if((e=b(this,e,t,0)??l)===A)return;let s=this._$AH,i=e===l&&s!==l||e.capture!==s.capture||e.once!==s.once||e.passive!==s.passive,n=e!==l&&(s===l||i);i&&this.element.removeEventListener(this.name,this,s),n&&this.element.addEventListener(this.name,this,e),this._$AH=e}handleEvent(e){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,e):this._$AH.handleEvent(e)}},W=class{constructor(e,t,s){this.element=e,this.type=6,this._$AN=void 0,this._$AM=t,this.options=s}get _$AU(){return this._$AM._$AU}_$AI(e){b(this,e)}};var Ce=K.litHtmlPolyfillSupport;Ce?.(U,O),(K.litHtmlVersions??=[]).push("3.3.3");var fe=(r,e,t)=>{let s=t?.renderBefore??e,i=s._$litPart$;if(i===void 0){let n=t?.renderBefore??null;s._$litPart$=i=new O(e.insertBefore(C(),n),n,void 0,t??{})}return i._$AI(r),i};var Z=globalThis,$=class extends m{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){let e=super.createRenderRoot();return this.renderOptions.renderBefore??=e.firstChild,e}update(e){let t=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(e),this._$Do=fe(t,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return A}};$._$litElement$=!0,$.finalized=!0,Z.litElementHydrateSupport?.({LitElement:$});var Pe=Z.litElementPolyfillSupport;Pe?.({LitElement:$});(Z.litElementVersions??=[]).push("4.2.2");function Ue(r){if(!r||typeof r!="object")return!1;let e=r;return typeof e.id=="string"&&/^occurrence-[\w-]+-\d+-\d+$/.test(e.id)&&typeof e.bookTitle=="string"&&e.bookTitle.length>0&&typeof e.left=="string"&&typeof e.surface=="string"&&e.surface.length>0&&typeof e.right=="string"&&typeof e.sentence=="string"&&Number.isInteger(e.targetStart)&&Number.isInteger(e.targetEnd)&&e.targetStart>=0&&e.targetEnd>e.targetStart&&e.targetEnd<=e.sentence.length&&e.sentence.slice(e.targetStart,e.targetEnd)===e.surface&&typeof e.studyUrl=="string"&&e.studyUrl.startsWith("/vocabulary/concordance/sentence?")}var G=class extends ${static styles=D`
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
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      .concordance-before { grid-column: 1; white-space: normal; }
      .concordance-surface { grid-column: 2; grid-row: 2; }
      .concordance-after { grid-column: 1 / -1; white-space: normal; }
      .concordance-study-link {
        align-self: start;
        min-width: 0;
        padding-inline: var(--mouseion-space-1, 0.25rem);
      }
    }
  `;occurrences=null;connectedCallback(){super.connectedCallback(),window.addEventListener("hashchange",this.restoreFragmentFocus),window.addEventListener("popstate",this.restoreFragmentFocus);try{let e=this.parentElement,t=document.getElementById("concordance-native-results");if(!e||!t)return;let s=JSON.parse(e.getAttribute("data-concordance-data")??"");if(!s||typeof s!="object"||!Array.isArray(s.occurrences)||!s.occurrences.every(Ue)||s.occurrences.length===0)return;this.occurrences=s.occurrences,this.requestUpdate(),this.updateComplete.then(()=>{!this.isConnected||!this.occurrences||(t.remove(),e.hidden=!1,this.restoreFragmentFocus())})}catch{}}disconnectedCallback(){window.removeEventListener("hashchange",this.restoreFragmentFocus),window.removeEventListener("popstate",this.restoreFragmentFocus),super.disconnectedCallback()}restoreFragmentFocus=()=>{if(!this.isConnected||!this.occurrences||!location.hash)return;let e=location.hash.slice(1);(this.shadowRoot?.getElementById(e)??document.getElementById("concordance-summary"))?.focus({preventScroll:!0})};render(){return this.occurrences?N`<ol class="concordance-results">
      ${this.occurrences.map(e=>N`<li class="concordance-result">
        <details class="concordance-row" id=${e.id} tabindex="-1">
          <summary aria-label=${`Occurrence of ${e.surface} in ${e.bookTitle}`} @click=${this.onRowClick}>
            <span class="concordance-book-title">${e.bookTitle}</span>
            <span class="concordance-before">${e.left}</span>
            <strong class="concordance-surface">${e.surface}</strong>
            <span class="concordance-after">${e.right}</span>
          </summary>
          <div class="concordance-context"><p class="reading-text">${this.sentence(e)}</p></div>
        </details>
        <a class="concordance-study-link" aria-label="Study this sentence and its syntax" title="Study this sentence and its syntax" href=${e.studyUrl}>
          <span aria-hidden="true">↗</span>
        </a>
      </li>`)}
    </ol>`:l}sentence(e){return N`${e.sentence.slice(0,e.targetStart)}<strong class="concordance-observed-target">${e.surface}</strong>${e.sentence.slice(e.targetEnd)}`}onRowClick=e=>{let t=e.currentTarget;if(!(t instanceof HTMLElement)||t.tagName!=="SUMMARY")return;let s=t.closest("details.concordance-row");!s||s.open||this.shadowRoot?.querySelectorAll("details.concordance-row[open]").forEach(i=>{i!==s&&(i.open=!1)})}};customElements.get("mouseion-concordance")||customElements.define("mouseion-concordance",G);
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

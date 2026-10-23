import DOMPurify from 'dompurify'

/** Only use this document in an opaque-origin sandboxed iframe, never v-html. */
export function qualityPreviewDocument(raw: string, nonce = ''): string {
  // Preserve styles, SVG and inline drawing scripts used by generated artwork.
  // The sandbox prevents access to the app; CSP denies network/subframes/forms.
  const clean = DOMPurify.sanitize(raw, {
    WHOLE_DOCUMENT: true,
    // SVG artwork reuses definitions (e.g. bicycle wheels) through <use>.
    // DOMPurify drops it by default; retain it and restrict both href forms below.
    ADD_TAGS: ['script', 'use'],
    FORBID_TAGS: ['iframe', 'object', 'embed', 'base', 'meta', 'link', 'form'],
    FORBID_ATTR: ['srcdoc', 'action', 'formaction', 'target'],
  })
  const doc = new DOMParser().parseFromString(clean, 'text/html')
  for (const script of doc.querySelectorAll('script')) {
    if (script.hasAttribute('src')) { script.remove(); continue }
    if (nonce) script.setAttribute('nonce', nonce)
    else script.removeAttribute('nonce')
  }
  // Keep same-document SVG references, including the legacy xlink spelling.
  // External SVG references and navigation to the application are not allowed.
  for (const link of doc.querySelectorAll('[href], [xlink\\:href]')) {
    for (const attribute of ['href', 'xlink:href']) {
      const value = link.getAttribute(attribute)
      if (value !== null && !value.startsWith('#')) link.removeAttribute(attribute)
    }
  }
  const policy = doc.createElement('meta')
  policy.httpEquiv = 'Content-Security-Policy'
  policy.content = `default-src 'none'; script-src ${nonce ? `'nonce-${nonce}'` : "'unsafe-inline'"}; script-src-attr 'none'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'`
  doc.head.prepend(policy)
  const charset = doc.createElement('meta')
  charset.setAttribute('charset', 'utf-8')
  doc.head.prepend(charset)
  const viewport = doc.createElement('meta')
  viewport.name = 'viewport'
  viewport.content = 'width=device-width, initial-scale=1'
  doc.head.append(viewport)
  const fallback = doc.createElement('style')
  // The whole document is fitted below; overflow must never add iframe chrome.
  fallback.textContent = 'html{background:#fff;color:#111;overflow:hidden!important;scrollbar-width:none!important;-ms-overflow-style:none!important}body{margin:0;overflow:hidden!important;scrollbar-width:none!important;-ms-overflow-style:none!important}html::-webkit-scrollbar,body::-webkit-scrollbar{display:none;width:0;height:0}img,svg,canvas{max-width:100%}'
  doc.head.insertBefore(fallback, policy.nextSibling)
  // Transform the complete document without changing its layout viewport.
  // CSS zoom would reflow percentage widths and viewport-height layouts and
  // can leave the footer outside the canvas even after measuring it.
  const fitScript = doc.createElement('script')
  fitScript.setAttribute('data-sub2api-preview-fit', 'true')
  if (nonce) fitScript.setAttribute('nonce', nonce)
  fitScript.textContent = `(() => {
    const root = document.documentElement;
    const body = document.body;
    let scheduled = false;
    const fit = () => {
      scheduled = false;
      root.style.setProperty('transform', 'none', 'important');
      const bounds = body.getBoundingClientRect();
      const width = Math.max(innerWidth, root.scrollWidth, body.scrollWidth + bounds.left, bounds.right);
      const height = Math.max(innerHeight, root.scrollHeight, body.scrollHeight + bounds.top, bounds.bottom);
      const scale = Math.min(1, innerWidth / width, innerHeight / height);
      const x = (innerWidth - width * scale) / 2;
      const y = (innerHeight - height * scale) / 2;
      root.style.setProperty('transform-origin', '0 0', 'important');
      root.style.setProperty('transform', 'translate(' + x + 'px,' + y + 'px) scale(' + scale + ')', 'important');
    };
    const schedule = () => {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(fit);
    };
    const sizes = new ResizeObserver(schedule);
    sizes.observe(body);
    const content = new MutationObserver(schedule);
    content.observe(body, { childList: true, subtree: true, characterData: true });
    document.addEventListener('load', schedule, true);
    document.fonts.ready.then(schedule);
    addEventListener('resize', schedule, { passive: true });
    schedule();
  })();`
  doc.body.append(fitScript)
  return `<!doctype html>${doc.documentElement.outerHTML}`
}

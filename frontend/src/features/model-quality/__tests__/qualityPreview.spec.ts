import { describe, expect, it } from 'vitest'
import { qualityPreviewDocument } from '../qualityPreview'

describe('isolated artwork document', () => {
  it('preserves inline drawing scripts containing compact less-than comparisons in document order', () => {
    const setup = 'const colors = ["#284e51", "#edf2e9"];'
    const draw = `const canvas = document.getElementById('scene');
      const context = canvas.getContext('2d');
      for(let k=0;k<12;k++) { context.fillRect(k*10, 0, 8, 8); }
      if(canvas.width<960) { canvas.width = 960; }`
    const raw = `<html><head><script>${setup}</script></head><body>
      <canvas id="scene" width="960" height="600"></canvas><script>${draw}</script>
    </body></html>`
    const doc = new DOMParser().parseFromString(qualityPreviewDocument(raw, 'drawing-nonce'), 'text/html')
    const scripts = [...doc.querySelectorAll('script:not([data-sub2api-preview-fit])')]
    expect(scripts.map(script => script.textContent)).toEqual([setup, draw])
    expect(scripts.map(script => script.getAttribute('nonce'))).toEqual(['drawing-nonce', 'drawing-nonce'])
    expect(doc.head.querySelector('script')?.textContent).toBe(setup)
    expect(doc.querySelector('canvas')?.nextElementSibling?.textContent).toBe(draw)
  })

  it('discards external and foreign-namespace scripts before restoring inline code', () => {
    const raw = `<html><body>
      <script src="https://outside.test/draw.js">externalFallback()</script>
      <script src="javascript:alert(1)">unsafeFallback()</script>
      <script src="">emptySourceFallback()</script>
      <svg xmlns="http://www.w3.org/2000/svg"><script>foreignScript()</script></svg>
      <canvas id="scene" onclick="alert(1)"></canvas>
      <script type="module" nonce="untrusted" onload="alert(1)">for(let i=0;i<5;i++) { console.log(i); }</script>
    </body></html>`
    const doc = new DOMParser().parseFromString(qualityPreviewDocument(raw, 'trusted'), 'text/html')
    const scripts = [...doc.querySelectorAll('script:not([data-sub2api-preview-fit])')]
    expect(scripts).toHaveLength(1)
    expect(scripts[0]?.textContent).toBe('for(let i=0;i<5;i++) { console.log(i); }')
    expect(scripts[0]?.getAttribute('type')).toBe('module')
    expect(scripts[0]?.getAttribute('nonce')).toBe('trusted')
    expect(doc.querySelector('[onclick], [onload], script[src], svg script')).toBeNull()
  })

  it('preserves local SVG references used for wheels, clouds and plants', () => {
    const raw = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 1000 670">
      <defs>
        <g id="wheel"><circle r="76" fill="none" stroke="#304d4b" /></g>
        <g id="cloud"><path d="M0 24Q30 0 60 24Z" /></g>
        <g id="grass"><path d="M0 0L8 -33" /></g>
      </defs>
      <g id="rear-wheel" transform="translate(350 477)"><use href="#wheel" /></g>
      <g id="front-wheel" transform="translate(660 477)"><use xlink:href="#wheel" /></g>
      <use href="#cloud" x="440" y="90" transform="scale(.8)" />
      <use href="#grass" x="112" y="494" />
    </svg>`
    const doc = new DOMParser().parseFromString(qualityPreviewDocument(raw), 'text/html')
    expect(doc.querySelectorAll('svg use')).toHaveLength(4)
    expect(doc.querySelector('#wheel circle')?.getAttribute('r')).toBe('76')
    expect(doc.querySelector('#rear-wheel use')?.getAttribute('href')).toBe('#wheel')
    expect(doc.querySelector('#front-wheel use')?.getAttribute('xlink:href')).toBe('#wheel')
    expect(doc.querySelector('use[href="#cloud"]')?.getAttribute('transform')).toBe('scale(.8)')
    expect(doc.querySelector('use[href="#grass"]')?.getAttribute('y')).toBe('494')
  })

  it('removes non-fragment SVG references without dropping their local siblings', () => {
    const raw = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">
      <defs><circle id="wheel" r="10" /></defs>
      <use id="remote" href="https://outside.test/image.svg#wheel" />
      <use id="relative" href="/api/private.svg#wheel" />
      <use id="legacy" xlink:href="https://outside.test/image.svg#wheel" />
      <use id="scripted" href="javascript:alert(1)" onload="alert(1)" />
      <use id="mixed" href="#wheel" xlink:href="//outside.test/image.svg#wheel" />
      <use id="local" href="#wheel" />
    </svg>`
    const doc = new DOMParser().parseFromString(qualityPreviewDocument(raw), 'text/html')
    expect(doc.querySelectorAll('use')).toHaveLength(6)
    for (const id of ['remote', 'relative', 'legacy', 'scripted']) {
      expect(doc.getElementById(id)?.hasAttribute('href')).toBe(false)
      expect(doc.getElementById(id)?.hasAttribute('xlink:href')).toBe(false)
    }
    expect(doc.querySelector('#scripted')?.hasAttribute('onload')).toBe(false)
    expect(doc.querySelector('#mixed')?.hasAttribute('xlink:href')).toBe(false)
    expect(doc.querySelector('#mixed')?.getAttribute('href')).toBe('#wheel')
    expect(doc.querySelector('#local')?.getAttribute('href')).toBe('#wheel')
  })

  it('preserves drawing styles, SVG and inline animation while removing outside embeds', () => {
    const raw = '<html><head><style>svg{background:#abc}</style></head><body><svg viewBox="0 0 10 10"><circle r="4"/></svg><script>document.body.dataset.drawn="yes"</script><script src="https://outside.test/a.js"></script><iframe src="/admin"></iframe><form action="/admin"><button>submit</button></form><meta http-equiv="refresh" content="0;url=/admin"><a href="/admin">link</a></body></html>'
    const doc = new DOMParser().parseFromString(qualityPreviewDocument(raw, 'test-nonce'), 'text/html')
    expect(doc.querySelector('svg circle')).not.toBeNull()
    expect(doc.querySelector('style')?.textContent).toContain('background')
    expect(doc.querySelectorAll('script')).toHaveLength(2)
    expect(doc.querySelector('script:not([data-sub2api-preview-fit])')?.getAttribute('nonce')).toBe('test-nonce')
    expect(doc.querySelector('script[data-sub2api-preview-fit]')?.getAttribute('nonce')).toBe('test-nonce')
    expect(doc.querySelector('iframe,form,script[src],a[href]')).toBeNull()
    expect(doc.querySelector('meta[http-equiv="refresh"]')).toBeNull()
    const policy = doc.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute('content')
    expect(policy).toContain("connect-src 'none'")
    expect(policy).toContain("frame-src 'none'")
    expect(policy).toContain("script-src 'nonce-test-nonce'")
    const fallback = [...doc.querySelectorAll('style')].find(style => style.textContent?.includes('overflow:hidden'))
    expect(fallback?.textContent).toContain('scrollbar-width:none')
    expect(fallback?.textContent).toContain('::-webkit-scrollbar')
  })
})

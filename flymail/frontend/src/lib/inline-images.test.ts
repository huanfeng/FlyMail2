import { describe, it, expect } from 'vitest'
import {
  CID_PATTERN,
  DRAFT_MAX_BYTES,
  collectImageSrcs,
  dataUriToFile,
  fileToDataUri,
  htmlByteSize,
  mapImageSrc,
  newCid,
  parseDataUri,
  prepareInlineForDraft,
  prepareInlineForSend,
  stripLocalImages,
} from '@/lib/inline-images'
import type { InlineAsset } from '@/lib/inline-images'

/** 1×1 透明 PNG */
const PNG_DATA_URI =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=='

function asset(cid: string): InlineAsset {
  return { cid, file: new File([new Uint8Array([1, 2, 3])], `${cid}.png`, { type: 'image/png' }) }
}

describe('newCid', () => {
  it('满足后端的 Content-ID 校验', () => {
    for (let i = 0; i < 50; i++) expect(CID_PATTERN.test(newCid())).toBe(true)
  })

  it('带 ii_ 前缀且不重复', () => {
    const set = new Set(Array.from({ length: 200 }, () => newCid()))
    expect(set.size).toBe(200)
    for (const c of set) expect(c.startsWith('ii_')).toBe(true)
  })

  it('CID_PATTERN 拒绝含 CRLF 的取值（头注入边界）', () => {
    expect(CID_PATTERN.test('ok_1')).toBe(true)
    expect(CID_PATTERN.test('a\r\nX-Evil: 1')).toBe(false)
    expect(CID_PATTERN.test('a<b>')).toBe(false)
    expect(CID_PATTERN.test('')).toBe(false)
  })
})

describe('collectImageSrcs / mapImageSrc', () => {
  const html = '<p><img src="blob:a"><b>x</b><img src="https://x/1.png"><img src="blob:b"></p>'

  it('按文档顺序列出 src', () => {
    expect(collectImageSrcs(html)).toEqual(['blob:a', 'https://x/1.png', 'blob:b'])
  })

  it('map 返回 null 的图原样保留', () => {
    const out = mapImageSrc(html, (src) => (src.startsWith('blob:') ? 'cid:x' : null))
    expect(out).toContain('src="cid:x"')
    expect(out).toContain('https://x/1.png')
  })
})

describe('prepareInlineForSend', () => {
  it('blob 图改写成 cid 引用，cids 与 files 严格同序', () => {
    const map: Record<string, InlineAsset> = { 'blob:a': asset('ii_a'), 'blob:b': asset('ii_b') }
    const html = '<p><img src="blob:a"></p><p><img src="blob:b"></p>'
    const out = prepareInlineForSend(html, (src) => map[src] ?? null)

    expect(out.html).toContain('src="cid:ii_a"')
    expect(out.html).toContain('src="cid:ii_b"')
    expect(out.cids).toEqual(['ii_a', 'ii_b'])
    expect(out.files.map((f) => f.name)).toEqual(['ii_a.png', 'ii_b.png'])
  })

  it('同一张图被引用两次只上传一份，第二次复用同一个 cid', () => {
    const map: Record<string, InlineAsset> = { 'blob:a': asset('ii_a') }
    const out = prepareInlineForSend(
      '<img src="blob:a"><img src="blob:a">',
      (src) => map[src] ?? null,
    )
    expect(out.cids).toEqual(['ii_a'])
    expect(out.files).toHaveLength(1)
    expect(out.html.match(/cid:ii_a/g)).toHaveLength(2)
  })

  it('远程图不动，也不进 files', () => {
    const out = prepareInlineForSend('<img src="https://x/1.png">', () => null)
    expect(out.html).toContain('https://x/1.png')
    expect(out.cids).toEqual([])
    expect(out.files).toEqual([])
  })

  it('查不到资源的本地图原样留下，不会产出对不上号的 cid', () => {
    const out = prepareInlineForSend('<img src="blob:missing">', () => null)
    expect(out.html).toContain('blob:missing')
    expect(out.cids).toEqual([])
  })

  it('cid 非法的资源一律拒绝（后端会因头注入 400）', () => {
    const bad: InlineAsset = { cid: 'a\r\nX: 1', file: new File([''], 'x.png') }
    const out = prepareInlineForSend('<img src="blob:a">', () => bad)
    expect(out.cids).toEqual([])
    expect(out.html).toContain('blob:a')
  })

  it('data: 图同样能收敛成 cid（草稿打开后直接发送的路径）', () => {
    const out = prepareInlineForSend(`<img src="${PNG_DATA_URI}">`, (src) => {
      const file = dataUriToFile(src, 'ii_d')
      return file ? { cid: 'ii_d', file } : null
    })
    expect(out.html).toContain('src="cid:ii_d"')
    expect(out.cids).toEqual(['ii_d'])
    expect(out.files[0].type).toBe('image/png')
  })
})

describe('data: URI 往返', () => {
  it('parseDataUri 解析 mime 与 base64', () => {
    const p = parseDataUri(PNG_DATA_URI)
    expect(p?.mime).toBe('image/png')
    expect(p?.base64.length).toBeGreaterThan(0)
  })

  it('非 base64 形式的 data: 一律拒绝', () => {
    expect(parseDataUri('data:text/plain,hello')).toBeNull()
    expect(parseDataUri('blob:abc')).toBeNull()
  })

  it('dataUriToFile 还原出的 File 类型与扩展名正确', () => {
    const f = dataUriToFile(PNG_DATA_URI, 'ii_abc')
    expect(f).not.toBeNull()
    expect(f?.name).toBe('ii_abc.png')
    expect(f?.type).toBe('image/png')
    expect((f as File).size).toBeGreaterThan(0)
  })

  it('jpeg 的扩展名归一成 jpg', () => {
    const f = dataUriToFile('data:image/jpeg;base64,AAAA', 'ii_j')
    expect(f?.name).toBe('ii_j.jpg')
  })

  it('File → data: → File 内容不变', async () => {
    const original = dataUriToFile(PNG_DATA_URI, 'ii_r') as File
    const uri = await fileToDataUri(original)
    const back = dataUriToFile(uri, 'ii_r') as File
    expect(back.size).toBe(original.size)
    expect(back.type).toBe(original.type)
    expect(await back.text()).toBe(await original.text())
  })
})

describe('草稿内嵌与截断', () => {
  it('blob 图换成 data: URI，远程图不动', async () => {
    const html = '<img src="blob:a"><img src="https://x/1.png">'
    const out = await prepareInlineForDraft(html, async (src) =>
      src === 'blob:a' ? PNG_DATA_URI : null,
    )
    expect(out.truncated).toBe(false)
    expect(out.html).toContain('data:image/png;base64,')
    expect(out.html).toContain('https://x/1.png')
    expect(out.html).not.toContain('blob:a')
  })

  it('超过 5 MiB 时丢掉内联图并报告截断', async () => {
    const huge = `data:image/png;base64,${'A'.repeat(DRAFT_MAX_BYTES + 1024)}`
    const out = await prepareInlineForDraft('<p>正文</p><img src="blob:a">', async () => huge)
    expect(out.truncated).toBe(true)
    expect(out.html).toContain('正文')
    expect(out.html).not.toContain('data:image/png')
  })

  it('stripLocalImages 只删本地图，远程图与正文保留', () => {
    const r = stripLocalImages('<p>t</p><img src="blob:a"><img src="https://x/1.png">')
    expect(r.removed).toBe(1)
    expect(r.html).toContain('https://x/1.png')
    expect(r.html).toContain('<p>t</p>')
  })

  it('htmlByteSize 按 UTF-8 字节算（中文一个字三字节）', () => {
    expect(htmlByteSize('abc')).toBe(3)
    expect(htmlByteSize('中')).toBe(3)
  })
})

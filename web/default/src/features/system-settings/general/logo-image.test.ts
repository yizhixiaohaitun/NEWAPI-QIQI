import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  ensureLogoImageDecodes,
  isSupportedLogoValue,
  logoBytesToDataUrl,
  MAX_LOGO_DIMENSION,
  MAX_LOGO_IMAGE_BYTES,
} from './logo-image.ts'

describe('logo image validation', () => {
  test('detects image bytes instead of trusting a file extension or declared type', () => {
    const png = Uint8Array.from([
      0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
    ])
    const value = logoBytesToDataUrl(png)
    assert.equal(value, 'data:image/png;base64,iVBORw0KGgo=')
    assert.equal(isSupportedLogoValue(value), true)
    assert.throws(() =>
      logoBytesToDataUrl(new TextEncoder().encode('<svg></svg>'))
    )
    assert.throws(() =>
      logoBytesToDataUrl(new TextEncoder().encode('<html></html>'))
    )
  })

  test('accepts legacy HTTP URLs and rejects active or malformed data URLs', () => {
    assert.equal(isSupportedLogoValue('https://example.com/logo.png'), true)
    assert.equal(isSupportedLogoValue('http://example.com/logo.webp'), true)
    assert.equal(isSupportedLogoValue('javascript:alert(1)'), false)
    assert.equal(
      isSupportedLogoValue('data:image/svg+xml;base64,PHN2Zz4='),
      false
    )
    assert.equal(isSupportedLogoValue('data:image/png;base64,PHN2Zz4='), false)
  })

  test('requires the browser decoder to accept the selected image', async () => {
    await ensureLogoImageDecodes('data:image/png;base64,valid', () => ({
      src: '',
      decode: async () => undefined,
      naturalWidth: 1,
      naturalHeight: 1,
    }))
    await assert.rejects(() =>
      ensureLogoImageDecodes('data:image/png;base64,truncated', () => ({
        src: '',
        decode: async () => {
          throw new Error('decode failed')
        },
        naturalWidth: 0,
        naturalHeight: 0,
      }))
    )
  })

  test('rejects decoded images with excessive dimensions', async () => {
    await assert.rejects(
      () =>
        ensureLogoImageDecodes('data:image/png;base64,large', () => ({
          src: '',
          decode: async () => undefined,
          naturalWidth: MAX_LOGO_DIMENSION + 1,
          naturalHeight: 1,
        })),
      /4096 x 4096/
    )
  })

  test('rejects empty and oversized uploads', () => {
    assert.throws(() => logoBytesToDataUrl(new Uint8Array()))
    const oversized = new Uint8Array(MAX_LOGO_IMAGE_BYTES + 1)
    oversized.set([0xff, 0xd8, 0xff])
    assert.throws(() => logoBytesToDataUrl(oversized))
  })
})

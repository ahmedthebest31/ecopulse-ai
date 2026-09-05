import { describe, expect, it } from 'vitest'
import { isValidEmail, isValidWebhookUrl } from './validation'

describe('isValidEmail', () => {
  it('accepts common valid addresses', () => {
    expect(isValidEmail('user@example.com')).toBe(true)
    expect(isValidEmail('first.last@example.co.uk')).toBe(true)
    expect(isValidEmail('user+tag@sub.example.com')).toBe(true)
    expect(isValidEmail('user-name@example.io')).toBe(true)
  })

  it('trims surrounding whitespace before validating', () => {
    expect(isValidEmail('  user@example.com  ')).toBe(true)
    expect(isValidEmail('\tuser@example.com\n')).toBe(true)
  })

  it('rejects an empty string and whitespace-only input', () => {
    expect(isValidEmail('')).toBe(false)
    expect(isValidEmail('   ')).toBe(false)
  })

  it('rejects addresses without a domain dot', () => {
    expect(isValidEmail('user@example')).toBe(false)
    expect(isValidEmail('user@')).toBe(false)
  })

  it('rejects addresses with no local part or with double @', () => {
    expect(isValidEmail('@example.com')).toBe(false)
    expect(isValidEmail('user@@example.com')).toBe(false)
  })

  it('rejects embedded whitespace', () => {
    expect(isValidEmail('user name@example.com')).toBe(false)
    expect(isValidEmail('user@exa mple.com')).toBe(false)
  })

  it('leniently accepts a trailing dot in the domain (regex behavior)', () => {
    expect(isValidEmail('user@example.com.')).toBe(true)
  })
})

describe('isValidWebhookUrl', () => {
  it('accepts http and https URLs', () => {
    expect(isValidWebhookUrl('http://example.com')).toBe(true)
    expect(isValidWebhookUrl('https://example.com/hook?token=abc')).toBe(true)
    expect(isValidWebhookUrl('https://hooks.example.com/path')).toBe(true)
    expect(isValidWebhookUrl('http://localhost:3000/hook')).toBe(true)
  })

  it('trims surrounding whitespace before validating', () => {
    expect(isValidWebhookUrl('  https://example.com/hook  ')).toBe(true)
  })

  it('accepts uppercase scheme because URL lowercases the protocol', () => {
    expect(isValidWebhookUrl('HTTP://example.com')).toBe(true)
  })

  it('rejects non-http(s) schemes', () => {
    expect(isValidWebhookUrl('ftp://example.com')).toBe(false)
    expect(isValidWebhookUrl('ws://example.com')).toBe(false)
    expect(isValidWebhookUrl('mailto:user@example.com')).toBe(false)
  })

  it('rejects javascript URLs', () => {
    expect(isValidWebhookUrl('javascript:alert(1)')).toBe(false)
  })

  it('rejects protocol-relative and scheme-less inputs', () => {
    expect(isValidWebhookUrl('//example.com/path')).toBe(false)
    expect(isValidWebhookUrl('example.com')).toBe(false)
  })

  it('rejects empty strings', () => {
    expect(isValidWebhookUrl('')).toBe(false)
    expect(isValidWebhookUrl('   ')).toBe(false)
  })

  it('rejects a bare scheme with no host', () => {
    expect(isValidWebhookUrl('http://')).toBe(false)
  })
})
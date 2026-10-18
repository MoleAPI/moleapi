import {
  constants,
  createDecipheriv,
  generateKeyPairSync,
  privateDecrypt,
  webcrypto,
} from 'node:crypto'

import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  clearPasswordEncryptionCache,
  encryptPassword,
} from '../password-encryption'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

const keys = generateKeyPairSync('rsa', { modulusLength: 2048 })
const publicKey = keys.publicKey
  .export({ type: 'spki', format: 'pem' })
  .toString()

beforeEach(() => {
  clearPasswordEncryptionCache()
  vi.stubGlobal('crypto', webcrypto)
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { kid: 'test-key', public_key: publicKey } },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

it('encrypts a Unicode password in the existing RSA-OAEP format', async () => {
  const password = 'pāssword-安全🔐'
  const result = await encryptPassword(password)
  const plaintext = privateDecrypt(
    {
      key: keys.privateKey,
      padding: constants.RSA_PKCS1_OAEP_PADDING,
      oaepHash: 'sha256',
    },
    Buffer.from(result.password_encrypted, 'base64')
  )
  expect(plaintext.toString()).toBe(password)
  expect(result.encryption_key_id).toBe('test-key')
})

it('encrypts a long password in the existing authenticated hybrid format', async () => {
  const password = '安全🔐'.repeat(80)
  const result = await encryptPassword(password)
  const [version, wrapped, nonce, encrypted] =
    result.password_encrypted.split('.')
  expect(version).toBe('v2')
  const secret = privateDecrypt(
    {
      key: keys.privateKey,
      padding: constants.RSA_PKCS1_OAEP_PADDING,
      oaepHash: 'sha256',
      oaepLabel: Buffer.from('password-v2'),
    },
    Buffer.from(wrapped, 'base64')
  )
  const payload = Buffer.from(encrypted, 'base64')
  const decipher = createDecipheriv(
    'aes-256-gcm',
    secret,
    Buffer.from(nonce, 'base64')
  )
  decipher.setAAD(Buffer.from(`password-v2:${result.encryption_key_id}`))
  decipher.setAuthTag(payload.subarray(-16))
  expect(
    Buffer.concat([
      decipher.update(payload.subarray(0, -16)),
      decipher.final(),
    ]).toString()
  ).toBe(password)
})

it('rejects password submission when native encryption is unavailable', async () => {
  vi.stubGlobal('crypto', {
    getRandomValues: webcrypto.getRandomValues.bind(webcrypto),
  })
  await expect(encryptPassword('private-password')).rejects.toThrow()
})

it('rejects an invalid public key and fetches a fresh key on retry', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, data: { kid: 'invalid', public_key: 'invalid' } },
  })
  await expect(encryptPassword('private-password')).rejects.toThrow()
  expect((await encryptPassword('private-password')).encryption_key_id).toBe(
    'test-key'
  )
  expect(api.get).toHaveBeenCalledTimes(2)
})

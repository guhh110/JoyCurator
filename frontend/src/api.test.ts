import { describe, expect, it } from 'vitest'
import { bytes, episodeCode, parseEpisodes } from './api'
describe('manual episode mapping', () => {
  it('accepts merged episodes and Chinese separators', () =>
    expect(parseEpisodes('1，3-5')).toEqual([1, 3, 4, 5]))
  it.each(['', '0', '3-1', '1,1', 'a', '1-1001', '10000'])(
    'rejects invalid mapping %s',
    (value) => expect(() => parseEpisodes(value)).toThrow(),
  )
  it('preserves specials', () =>
    expect(episodeCode(0, [2, 3])).toBe('S00E02 · E03'))
  it('formats file sizes', () => expect(bytes(1024 ** 3)).toBe('1.0 GB'))
})

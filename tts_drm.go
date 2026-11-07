package main

// Sec-MS-GEC v2 令牌生成。
//
// 微软 Edge「大声朗读」接口自 2026 年 10 月起只接受 v2 令牌，
// 旧的“纯 SHA-256”v1 令牌会返回 HTTP 403。
//
// 本文件是 edge-tts（https://github.com/rany2/edge-tts）
// src/edge_tts/drm.py 中 v2 算法的 Go 移植，遵循 LGPL-3.0 许可。
// 本项目整体为 GPL-3.0，与 LGPL-3.0 兼容。

import (
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	drmWinEpoch   int64 = 11644473600
	drmWindowSecs int64 = 300
)

var (
	drmSHA256Initial = [8]uint32{
		0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
		0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
	}
	drmSHA256State = [8]uint32{
		0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A,
		0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
	}
	drmSHA256Schedule = [14]uint32{
		0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA,
		0xA622EE8C, 0xD1DD9491, 0xC0DD25A0, 0xF9E70AB6,
		0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33,
		0xD258B060, 0x72766564,
	}
	drmSHA256RoundConstants = [64]uint32{
		0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5,
		0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
		0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3,
		0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
		0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC,
		0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
		0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7,
		0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
		0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13,
		0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
		0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3,
		0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
		0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5,
		0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
		0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208,
		0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
	}
	drmSHA256BlockConstants = func() [64]uint32 {
		out := drmSHA256RoundConstants
		out[14] = drmSHA256Adjustment
		return out
	}()
	drmSHA256Adjustment   uint32 = 0xC9DC06A7
	drmSHA256Sigma0Adjust uint32 = 0x059C0B80
	drmSHA256Sigma1Adjust uint32 = 0x000B92C0
	drmCS64Coefficients          = [14][2]uint32{
		{0xE82E91B1, 0xB32700D0},
		{0x44DE24C6, 0x88DFF51D},
		{0x3BEB65BC, 0x29514038},
		{0xC89C5933, 0xB328CBDF},
		{0x8D1EC211, 0xD405B4D5},
		{0x314EF5D5, 0x26E55F3C},
		{0xE92491B1, 0x607801C6},
		{0xF0A5B9D5, 0x82B2C1FB},
		{0x06064CF3, 0xFA51A3DA},
		{0xDA79FA60, 0x207CC464},
		{0xBED20560, 0x3C3C74F3},
		{0x89F9C9F8, 0xD2D14B61},
		{0x35FA38C6, 0xBB04A2C1},
		{0xCB49E3C6, 0xC5626167},
	}
	drmCS64Factor      uint32 = 0x82893933
	drmCS64Adjustment0 uint32 = 0xF2F90B1A
	drmCS64Adjustment1 uint32 = 0x0B1A0000
)

// generateEdgeSecMSGec 生成当前时间的 Sec-MS-GEC v2 令牌。
func generateEdgeSecMSGec() string {
	return generateEdgeSecMSGecAt(time.Now().Unix())
}

// generateEdgeSecMSGecAt 生成指定 Unix 时间戳对应的 Sec-MS-GEC v2 令牌。
// 令牌格式为 "<16位十六进制>-<64位十六进制>"。
func generateEdgeSecMSGecAt(unix int64) string {
	ticks := unix + drmWinEpoch
	ticks -= ticks % drmWindowSecs
	payload := strconv.FormatInt(ticks*10000000, 10) + edgeTrustedClientToken
	cs64, sha256 := drmTokenHash([]byte(payload))
	return cs64 + "-" + sha256
}

// drmTokenHash 返回 CS64 与定制 SHA-256 两部分十六进制摘要。
func drmTokenHash(data []byte) (string, string) {
	digest := drmSHA256Digest(data)
	cs64a, cs64b := drmCS64Digest(data, digest)
	var cs64 [8]byte
	binary.LittleEndian.PutUint32(cs64[0:4], cs64a)
	binary.LittleEndian.PutUint32(cs64[4:8], cs64b)
	return strings.ToUpper(hex.EncodeToString(cs64[:])),
		strings.ToUpper(hex.EncodeToString(digest))
}

func drmRol16(value uint32) uint32 {
	return (value << 16) | (value >> 16)
}

func drmSigma0(value uint32) uint32 {
	return (value >> 7) ^ (value << 25) ^ (value >> 18) ^ (value << 14) ^ (value >> 3)
}

func drmSigma1(value uint32) uint32 {
	return (value >> 17) ^ (value << 15) ^ (value >> 19) ^ (value << 13) ^ (value >> 10)
}

// drmWordsLE4Even 把数据按小端 32 位切分，并保证字数为偶数。
func drmWordsLE4Even(data []byte) []uint32 {
	padded := data
	if rem := len(data) % 4; rem != 0 {
		padded = append(append([]byte{}, data...), make([]byte, 4-rem)...)
	}
	words := make([]uint32, 0, len(padded)/4+1)
	for i := 0; i < len(padded); i += 4 {
		words = append(words, binary.LittleEndian.Uint32(padded[i:i+4]))
	}
	if len(words)%2 == 1 {
		words = append(words, 0)
	}
	return words
}

func drmExpandSHA256Schedule(words []uint32, firstWord int) []uint32 {
	for index := firstWord; index < 64; index++ {
		x := words[index-2]
		y := words[index-15]
		sigma1 := drmSigma1(x)
		sigma0 := drmSigma0(y)
		words = append(words, sigma1+words[index-7]+sigma0+words[index-16])
	}
	return words
}

func drmSHA256Rounds(state [8]uint32, words []uint32, firstRound int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	constants := drmSHA256RoundConstants[:]
	if firstRound == 14 {
		constants = drmSHA256BlockConstants[:]
	}
	aligned := ((firstRound + 7) / 8) * 8
	if aligned > 64 {
		aligned = 64
	}
	for index := firstRound; index < aligned; index++ {
		rotE := uint64(e) * 0x100000001
		rotA := uint64(a) * 0x100000001
		t1 := uint64(h) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(g^(e&(f^g))) +
			uint64(constants[index]) +
			uint64(words[index])
		t2 := ((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((a&b)|(c&(a|b)))
		a, b, c, d, e, f, g, h = uint32((t1+t2)&0xFFFFFFFF), a, b, c, uint32((uint64(d)+t1)&0xFFFFFFFF), e, f, g
	}
	for index := aligned; index < 64; index += 8 {
		rotE := uint64(e) * 0x100000001
		rotA := uint64(a) * 0x100000001
		t1 := uint64(h) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(g^(e&(f^g))) +
			uint64(constants[index]) +
			uint64(words[index])
		d = uint32((uint64(d) + t1) & 0xFFFFFFFF)
		h = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((a&b)|(c&(a|b)))) & 0xFFFFFFFF)

		rotE = uint64(d) * 0x100000001
		rotA = uint64(h) * 0x100000001
		t1 = uint64(g) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(f^(d&(e^f))) +
			uint64(constants[index+1]) +
			uint64(words[index+1])
		c = uint32((uint64(c) + t1) & 0xFFFFFFFF)
		g = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((h&a)|(b&(h|a)))) & 0xFFFFFFFF)

		rotE = uint64(c) * 0x100000001
		rotA = uint64(g) * 0x100000001
		t1 = uint64(f) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(e^(c&(d^e))) +
			uint64(constants[index+2]) +
			uint64(words[index+2])
		b = uint32((uint64(b) + t1) & 0xFFFFFFFF)
		f = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((g&h)|(a&(g|h)))) & 0xFFFFFFFF)

		rotE = uint64(b) * 0x100000001
		rotA = uint64(f) * 0x100000001
		t1 = uint64(e) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(d^(b&(c^d))) +
			uint64(constants[index+3]) +
			uint64(words[index+3])
		a = uint32((uint64(a) + t1) & 0xFFFFFFFF)
		e = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((f&g)|(h&(f|g)))) & 0xFFFFFFFF)

		rotE = uint64(a) * 0x100000001
		rotA = uint64(e) * 0x100000001
		t1 = uint64(d) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(c^(a&(b^c))) +
			uint64(constants[index+4]) +
			uint64(words[index+4])
		h = uint32((uint64(h) + t1) & 0xFFFFFFFF)
		d = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((e&f)|(g&(e|f)))) & 0xFFFFFFFF)

		rotE = uint64(h) * 0x100000001
		rotA = uint64(d) * 0x100000001
		t1 = uint64(c) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(b^(h&(a^b))) +
			uint64(constants[index+5]) +
			uint64(words[index+5])
		g = uint32((uint64(g) + t1) & 0xFFFFFFFF)
		c = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((d&e)|(f&(d|e)))) & 0xFFFFFFFF)

		rotE = uint64(g) * 0x100000001
		rotA = uint64(c) * 0x100000001
		t1 = uint64(b) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(a^(g&(h^a))) +
			uint64(constants[index+6]) +
			uint64(words[index+6])
		f = uint32((uint64(f) + t1) & 0xFFFFFFFF)
		b = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((c&d)|(e&(c|d)))) & 0xFFFFFFFF)

		rotE = uint64(f) * 0x100000001
		rotA = uint64(b) * 0x100000001
		t1 = uint64(a) +
			((rotE >> 6) ^ (rotE >> 11) ^ (rotE >> 25)) +
			uint64(h^(f&(g^h))) +
			uint64(constants[index+7]) +
			uint64(words[index+7])
		e = uint32((uint64(e) + t1) & 0xFFFFFFFF)
		a = uint32((t1 +
			((rotA >> 2) ^ (rotA >> 13) ^ (rotA >> 22)) +
			uint64((b&c)|(d&(b|c)))) & 0xFFFFFFFF)
	}
	return [8]uint32{a, b, c, d, e, f, g, h}
}

func drmSHA256Init(data []byte) [8]uint32 {
	words := make([]uint32, 14, 64)
	words = append(words,
		uint32(data[0])<<16|uint32(data[1])<<8|uint32(data[2]),
		uint32(data[3])<<24|uint32(data[4])<<16|uint32(data[5])<<8|uint32(data[6]))
	for index := 16; index < 30; index++ {
		value := drmSHA256Schedule[index-16]
		if index-2 >= 14 {
			if index == 16 {
				value += drmSigma1(words[index-2]) ^ drmSHA256Sigma1Adjust
			} else {
				value += drmSigma1(words[index-2])
			}
		}
		if index-7 >= 14 {
			value += words[index-7]
		}
		if index-15 >= 14 {
			if index == 29 {
				value += drmSigma0(words[index-15]) ^ drmSHA256Sigma0Adjust
			} else {
				value += drmSigma0(words[index-15])
			}
		}
		words = append(words, value)
	}
	words = append(words,
		drmSigma1(words[28])+words[23]+drmSigma0(words[15])+words[14]+
			drmSHA256Adjustment-drmSHA256RoundConstants[14])
	words = drmExpandSHA256Schedule(words, 31)
	working := drmSHA256Rounds(drmSHA256State, words, 14)
	var result [8]uint32
	for i := range result {
		result[i] = drmSHA256Initial[i] + working[i]
	}
	return result
}

func drmSHA256Digest(data []byte) []byte {
	length := 57 + len(data)
	padded := make([]byte, 0, len(data)+64)
	padded = append(padded, data[7:]...)
	padded = append(padded, 0x80)
	padLen := ((55 - length) % 64)
	if padLen < 0 {
		padLen += 64
	}
	padded = append(padded, make([]byte, padLen)...)
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], uint64(length)*8)
	padded = append(padded, lengthBytes[:]...)

	state := drmSHA256Init(data[:7])
	for offset := 0; offset < len(padded); offset += 64 {
		words := make([]uint32, 16, 64)
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(padded[offset+i*4 : offset+i*4+4])
		}
		words = drmExpandSHA256Schedule(words, 16)
		working := drmSHA256Rounds(state, words, 0)
		for i := range state {
			state[i] += working[i]
		}
	}

	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:], state[i])
	}
	return out
}

func drmCS64Digest(data []byte, digest []byte) (uint32, uint32) {
	keyA := binary.LittleEndian.Uint32(digest[0:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1
	factors := [2]uint32{keyA * drmCS64Factor, keyB * drmCS64Factor}

	terms := make([]uint32, 0, len(drmCS64Coefficients)*2+len(data)/2)
	offsets := make([]uint32, 0, len(drmCS64Coefficients)*2+len(data)/2)
	for index, coefficient := range drmCS64Coefficients {
		terms = append(terms, coefficient[0]*factors[index%2])
		offsets = append(offsets, coefficient[1])
	}

	for index, word := range drmWordsLE4Even(append([]byte{0}, data...)) {
		key := keyA
		multiplier := uint32(0xEF0569FB)
		if index%2 == 1 {
			key = keyB
			multiplier = 0xC31713DB
		}
		term := word * key
		offset := drmRol16(word) * multiplier
		if index == 0 {
			term += drmCS64Adjustment0 * factors[0]
			offset += drmCS64Adjustment1
		}
		terms = append(terms, term)
		offsets = append(offsets, offset)
	}

	var swapAcc, swapLast, state, acc uint32
	for index := 0; index < len(terms); index += 2 {
		x, y := terms[index], terms[index+1]

		t := drmRol16(offsets[index] + x)
		q := t * 0x76F50F34
		swapAcc += drmRol16(q)*0xC3EFEA97 + q
		t = drmRol16(offsets[index+1] + y)
		q = t * 0x1881CE3C
		swapLast = drmRol16(q)*0x35BD1EC9 + q
		swapAcc += swapLast

		state = state*keyA + x
		state = drmRol16(state) * 0xCF98B111
		state = drmRol16(state) * 0x87085B9F
		state = drmRol16(state) * 0x12CEB96D
		state = drmRol16(state) * 0x257E1D83
		acc += state
		t = state*keyB + y
		t = drmRol16(t) * 0xA27416F5
		t = drmRol16(t) * 0xD38396FF
		state = drmRol16(drmRol16(t)*0x7C932B89) * 0xBFA49F69
		acc += state
	}
	return swapLast ^ state, swapAcc ^ acc
}

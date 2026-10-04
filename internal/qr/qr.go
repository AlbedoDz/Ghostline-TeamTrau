// Package qr is a minimal QR code encoder (byte mode, error correction
// level M, versions 1–10) for showing the LAN proxy address. The algorithm
// follows ISO/IEC 18004; structure after Project Nayuki's reference encoder.
package qr

import (
	"errors"
)

// block layout for level M: data codewords per block in group 1 and 2.
type layout struct {
	ecPerBlock     int
	g1Blocks, g1DC int
	g2Blocks, g2DC int
}

var layouts = [11]layout{
	{}, // unused
	{10, 1, 16, 0, 0},
	{16, 1, 28, 0, 0},
	{26, 1, 44, 0, 0},
	{18, 2, 32, 0, 0},
	{24, 2, 43, 0, 0},
	{16, 4, 27, 0, 0},
	{18, 4, 31, 0, 0},
	{22, 2, 38, 2, 39},
	{22, 3, 36, 2, 37},
	{26, 4, 43, 1, 44},
}

var alignment = [11][]int{
	{}, {}, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34},
	{6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
}

const maxVersion = 10

// ErrTooLong means text does not fit in version 10 at level M.
var ErrTooLong = errors.New("qr: text too long")

func (l layout) dataCodewords() int { return l.g1Blocks*l.g1DC + l.g2Blocks*l.g2DC }

// Encode returns the module matrix (true = dark), without quiet zone.
func Encode(text string) ([][]bool, error) {
	data := []byte(text)
	ver := 0
	for v := 1; v <= maxVersion; v++ {
		countBits := 8
		if v >= 10 {
			countBits = 16
		}
		if 4+countBits+8*len(data) <= layouts[v].dataCodewords()*8 {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, ErrTooLong
	}
	cw := addECC(encodeData(data, ver), ver)
	q := newSymbol(ver)
	q.drawFunctionPatterns()
	q.drawCodewords(cw)
	best, bestPenalty := 0, -1
	for mask := 0; mask < 8; mask++ {
		q.applyMask(mask)
		q.drawFormat(mask)
		if p := q.penalty(); bestPenalty < 0 || p < bestPenalty {
			best, bestPenalty = mask, p
		}
		q.applyMask(mask) // undo (XOR)
	}
	q.applyMask(best)
	q.drawFormat(best)
	return q.modules, nil
}

// encodeData builds the data codeword sequence: mode, count, bytes,
// terminator and padding.
func encodeData(data []byte, ver int) []byte {
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (v>>i)&1 == 1)
		}
	}
	put(0b0100, 4)
	if ver >= 10 {
		put(len(data), 16)
	} else {
		put(len(data), 8)
	}
	for _, b := range data {
		put(int(b), 8)
	}
	capBits := layouts[ver].dataCodewords() * 8
	for i := 0; i < 4 && len(bits) < capBits; i++ {
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	for pad := 0xEC; len(bits) < capBits; pad ^= 0xEC ^ 0x11 {
		put(pad, 8)
	}
	out := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b {
			out[i/8] |= 1 << (7 - uint(i%8))
		}
	}
	return out
}

// addECC splits data into blocks, appends Reed–Solomon codewords and
// interleaves the result.
func addECC(data []byte, ver int) []byte {
	l := layouts[ver]
	div := rsDivisor(l.ecPerBlock)
	var blocks, ecs [][]byte
	off := 0
	for i := 0; i < l.g1Blocks+l.g2Blocks; i++ {
		n := l.g1DC
		if i >= l.g1Blocks {
			n = l.g2DC
		}
		b := data[off : off+n]
		off += n
		blocks = append(blocks, b)
		ecs = append(ecs, rsRemainder(b, div))
	}
	var out []byte
	for i := 0; i < max(l.g1DC, l.g2DC); i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < l.ecPerBlock; i++ {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}
	return out
}

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int((y>>uint(i))&1) * int(x)
	}
	return byte(z)
}

func rsDivisor(degree int) []byte {
	r := make([]byte, degree)
	r[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			r[j] = gfMul(r[j], root)
			if j+1 < degree {
				r[j] ^= r[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return r
}

func rsRemainder(data, div []byte) []byte {
	r := make([]byte, len(div))
	for _, b := range data {
		f := b ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i := range r {
			r[i] ^= gfMul(div[i], f)
		}
	}
	return r
}

type symbol struct {
	ver      int
	size     int
	modules  [][]bool
	function [][]bool
}

func newSymbol(ver int) *symbol {
	n := 17 + 4*ver
	s := &symbol{ver: ver, size: n, modules: make([][]bool, n), function: make([][]bool, n)}
	for i := range s.modules {
		s.modules[i] = make([]bool, n)
		s.function[i] = make([]bool, n)
	}
	return s
}

// set marks a function module at column x, row y.
func (s *symbol) set(x, y int, dark bool) {
	s.modules[y][x] = dark
	s.function[y][x] = true
}

func (s *symbol) drawFunctionPatterns() {
	for i := 0; i < s.size; i++ {
		s.set(6, i, i%2 == 0)
		s.set(i, 6, i%2 == 0)
	}
	s.finder(3, 3)
	s.finder(s.size-4, 3)
	s.finder(3, s.size-4)
	al := alignment[s.ver]
	for i, ax := range al {
		for j, ay := range al {
			if i == 0 && j == 0 || i == 0 && j == len(al)-1 || i == len(al)-1 && j == 0 {
				continue // overlaps a finder
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					s.set(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	s.drawFormat(0) // reserve the format areas
	if s.ver >= 7 {
		rem := s.ver
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := s.ver<<12 | rem
		for i := 0; i < 18; i++ {
			dark := (bits>>uint(i))&1 == 1
			a, b := s.size-11+i%3, i/3
			s.set(a, b, dark)
			s.set(b, a, dark)
		}
	}
}

func (s *symbol) finder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= s.size || y >= s.size {
				continue
			}
			d := max(abs(dx), abs(dy))
			s.set(x, y, d != 2 && d != 4)
		}
	}
}

// drawFormat writes the 15-bit format information (level M = 0b00).
func (s *symbol) drawFormat(mask int) {
	data := 0<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>uint(i))&1 == 1 }
	for i := 0; i <= 5; i++ {
		s.set(8, i, bit(i))
	}
	s.set(8, 7, bit(6))
	s.set(8, 8, bit(7))
	s.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		s.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		s.set(s.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		s.set(8, s.size-15+i, bit(i))
	}
	s.set(8, s.size-8, true) // dark module
}

func (s *symbol) drawCodewords(cw []byte) {
	i := 0
	for right := s.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < s.size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = s.size - 1 - vert
				}
				if !s.function[y][x] && i < len(cw)*8 {
					s.modules[y][x] = (cw[i>>3]>>(7-uint(i&7)))&1 == 1
					i++
				}
			}
		}
	}
}

func (s *symbol) applyMask(mask int) {
	for y := 0; y < s.size; y++ {
		for x := 0; x < s.size; x++ {
			if s.function[y][x] {
				continue
			}
			var inv bool
			switch mask {
			case 0:
				inv = (x+y)%2 == 0
			case 1:
				inv = y%2 == 0
			case 2:
				inv = x%3 == 0
			case 3:
				inv = (x+y)%3 == 0
			case 4:
				inv = (x/3+y/2)%2 == 0
			case 5:
				inv = x*y%2+x*y%3 == 0
			case 6:
				inv = (x*y%2+x*y%3)%2 == 0
			case 7:
				inv = ((x+y)%2+x*y%3)%2 == 0
			}
			if inv {
				s.modules[y][x] = !s.modules[y][x]
			}
		}
	}
}

// penalty scores the symbol with the four ISO mask rules.
func (s *symbol) penalty() int {
	n := s.size
	at := func(x, y int, col bool) bool {
		if col {
			return s.modules[x][y]
		}
		return s.modules[y][x]
	}
	p := 0
	finder := []bool{true, false, true, true, true, false, true}
	for _, col := range []bool{false, true} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, col) == at(x-1, y, col) {
					run++
					continue
				}
				if run >= 5 {
					p += 3 + run - 5
				}
				run = 1
			}
			// 1:1:3:1:1 with 4 light modules on one side.
			for x := 0; x+7 <= n; x++ {
				match := true
				for k, f := range finder {
					if at(x+k, y, col) != f {
						match = false
						break
					}
				}
				if !match {
					continue
				}
				light := func(from, to int) bool {
					for k := from; k < to; k++ {
						if k >= 0 && k < n && at(k, y, col) {
							return false
						}
					}
					return true
				}
				if light(x-4, x) || light(x+7, x+11) {
					p += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if s.modules[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := s.modules[y][x]
				if c == s.modules[y][x+1] && c == s.modules[y+1][x] && c == s.modules[y+1][x+1] {
					p += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return p + k*10
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

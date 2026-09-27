package jam

// JAM CRC-32. Polynomial edb88320, seed ffffffff, no final complement.
// Strings are lowercased A-Z before the CRC, per the JAM spec.
func CRC32(b []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		crc = crcTable[(crc^uint32(c))&0xFF] ^ (crc >> 8)
	}
	return crc
}

func CRC32String(s string) uint32 {
	return CRC32([]byte(s))
}

var crcTable = makeCRCTable()

func makeCRCTable() [256]uint32 {
	var t [256]uint32
	for i := 0; i < 256; i++ {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
		t[i] = crc
	}
	return t
}

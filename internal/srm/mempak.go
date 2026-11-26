package srm

// Controller Pak note table: 16 entries of 32 bytes at pak offset 0x300.
const (
	noteTableOff = 0x300
	noteCount    = 16
	noteSize     = 32
)

// Note is one used entry in a Controller Pak's note table.
type Note struct {
	Index     int      // 0-15
	GameCode  [4]byte  // bytes 0-3
	Publisher [2]byte  // bytes 4-5
	RawName   [16]byte // bytes 0x10-0x1F, N64 font encoding
}

// Notes returns the used entries of pak's note table. An entry is used if its
// game code is non-zero. pak must be at least 0x500 bytes long.
func Notes(pak []byte) []Note {
	var notes []Note
	for i := range noteCount {
		e := pak[noteTableOff+i*noteSize : noteTableOff+(i+1)*noteSize]
		if e[0]|e[1]|e[2]|e[3] == 0 {
			continue
		}
		n := Note{Index: i}
		copy(n.GameCode[:], e[0:4])
		copy(n.Publisher[:], e[4:6])
		copy(n.RawName[:], e[0x10:0x20])
		notes = append(notes, n)
	}
	return notes
}

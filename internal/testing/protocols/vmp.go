package protocols

// Virtual Modem Protocol (VMP) frame recognition.
//
// VMP is the binary protocol implemented by Ray Gwinn's OS/2 VMODEM.EXE, the
// software the IVM ("Internet VMODEM") nodelist flag actually announces. The
// frame format was recovered from VMODEM.EXE's send routine and confirmed
// byte-for-byte against live OS/2 nodes:
//
//	10 02            frame marker (DLE STX), literal, never stuffed
//	<len:16 BE>      payload length, DLE-stuffed
//	<payload>        `len` bytes, DLE-stuffed; starts with a 16-bit BE command
//
// Any 0x10 (DLE) byte inside the length or payload is escaped by doubling.
// Example disconnect frame (command 1, reason 8): 10 02 00 04 00 01 00 08.
//
// We only need to *recognize* a well-formed VMP frame to confirm a genuine
// VMODEM responder — not to run a full VMP session.

const (
	vmpDLE = 0x10
	vmpSTX = 0x02
)

// vmpMaxCommand bounds a plausible command word. VMODEM.EXE emits small command
// codes (1=disconnect, 3=connect/ring, ...) and elsewhere guards a related
// field against values >= 0x15, so anything past a small range is not VMP.
const vmpMaxCommand = 0x14

// unstuffDLE collapses doubled DLE (0x10 0x10 -> 0x10). A trailing lone DLE
// (an escape whose second half hasn't arrived yet) is dropped.
func unstuffDLE(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == vmpDLE {
			if i+1 < len(b) && b[i+1] == vmpDLE {
				out = append(out, vmpDLE)
				i++
				continue
			}
			if i+1 >= len(b) {
				break // incomplete escape at end
			}
			// A lone DLE not followed by DLE shouldn't occur inside a valid
			// frame body; keep it so a malformed frame fails validation below.
			out = append(out, vmpDLE)
			continue
		}
		out = append(out, b[i])
	}
	return out
}

// looksLikeVMP reports whether b begins with a well-formed VMP frame and, if so,
// the command word carried in its payload. It tolerates a truncated read as long
// as the marker, length and command word are present.
func looksLikeVMP(b []byte) (ok bool, command int) {
	if len(b) < 2 || b[0] != vmpDLE || b[1] != vmpSTX {
		return false, 0
	}
	rest := unstuffDLE(b[2:])
	if len(rest) < 4 {
		return false, 0 // need length word + command word
	}
	length := int(rest[0])<<8 | int(rest[1])
	command = int(rest[2])<<8 | int(rest[3])
	if length < 2 || length > 512 {
		return false, 0
	}
	if command < 1 || command > vmpMaxCommand {
		return false, 0
	}
	return true, command
}

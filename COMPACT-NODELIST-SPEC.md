# Compact Nodelist Format (CNL) Specification v1.0

## Design Goals

- **Minimal wire size** - Every bit counts on slow/expensive links
- **CPU tradeoff accepted** - Use aggressive compression, decompressors can use all available CPU
- **Machine-readable only** - No human readability requirements
- **Self-contained** - Single file includes all data and dictionaries
- **Streamable** - Can decompress and process incrementally if needed

## Size Comparison (Estimated)

| Format | Typical Nodelist Size | Compression Ratio |
|--------|----------------------|-------------------|
| Original text | ~2.5 MB | 1x (baseline) |
| Text + gzip | ~400 KB | ~6x |
| Text + zstd | ~300 KB | ~8x |
| **CNL Binary + zstd** | **~80-120 KB** | **~20-30x** |

## File Structure

```
+--------------------------------------------------+
| MAGIC (4 bytes): "CNL\x01"                       |
+--------------------------------------------------+
| Header (variable)                                |
+--------------------------------------------------+
| String Dictionary (compressed block)             |
+--------------------------------------------------+
| Node Data (compressed block)                     |
+--------------------------------------------------+
| Checksum (4 bytes): CRC-32                       |
+--------------------------------------------------+
```

## Header Format

```
Offset  Size  Description
------  ----  -----------
0       2     Header size (little-endian)
2       2     Nodelist day number (1-366)
4       2     Nodelist year (e.g., 2024)
6       4     Original CRC from source nodelist
10      4     Total node count
14      2     String dictionary entry count
16      2     Compression algorithm ID:
                0x01 = zstd (preferred)
                0x02 = lzma
                0x03 = brotli
18      1     Protocol version (1)
19      1     Flags:
                bit 0: has extended internet config
                bit 1: has scheduling data
                bit 2-7: reserved
```

## Compression Strategy

Compression is applied in two layers:

1. **Semantic encoding** - Domain-specific binary encoding
2. **Entropy compression** - zstd level 19+ or lzma level 9

### Recommended: zstd with dictionary

Pre-trained dictionary on historical nodelists provides 15-25% additional compression.

```bash
# Training (one-time)
zstd --train nodelist.* -o nodelist.dict

# Compression
zstd -19 --long -D nodelist.dict input.cnl-raw -o output.cnl
```

## String Dictionary Block

All strings are deduplicated and stored in a single dictionary. Strings are referenced by 16-bit index (supports 65535 unique strings).

```
+--------------------------------------------------+
| Compressed block header                          |
| - Uncompressed size (4 bytes, varint)            |
| - Compressed size (4 bytes, varint)              |
+--------------------------------------------------+
| Compressed payload:                              |
| +----------------------------------------------+ |
| | Entry count (varint)                         | |
| +----------------------------------------------+ |
| | For each entry:                              | |
| |   - String length (varint, max 255)          | |
| |   - String bytes (UTF-8)                     | |
| +----------------------------------------------+ |
+--------------------------------------------------+
```

### String Categories (for optimal ordering)

Strings should be sorted into categories for better compression:

1. **Locations** (often share prefixes like "Stockholm_", "San_")
2. **Sysop names** (alphabetical helps compression)
3. **System names**
4. **Hostnames/INA values**
5. **Misc strings**

## Node Data Block

```
+--------------------------------------------------+
| Compressed block header                          |
| - Uncompressed size (4 bytes, varint)            |
| - Compressed size (4 bytes, varint)              |
+--------------------------------------------------+
| Compressed payload (see Node Record Format)      |
+--------------------------------------------------+
```

### Node Record Format

Each node is encoded as a variable-length record:

```
Byte 0: Record Type + Flags (1 byte)
  bits 0-2: Node type (see below)
  bit 3:    Has phone number (0 = unpublished)
  bit 4:    Has internet config
  bit 5:    Has extended flags
  bit 6:    Has scheduling data
  bit 7:    Has conflict (duplicate in source)

Bytes 1+: Node number (varint, delta-encoded within net)

If new Zone/Region/Net/Hub (type 0-3):
  - Zone (varint, only if Zone type or zone changed)
  - Net (varint, only if Net/Region type or net changed)
  - Region (varint, only if Region type)

String references (each is 16-bit index into dictionary):
  - System name index (2 bytes)
  - Location index (2 bytes)
  - Sysop name index (2 bytes)

If has phone (bit 3 = 1):
  - Phone string index (2 bytes)

Baud rate (4 bits) + Core flags (4 bits):
  bits 0-3: Baud rate index (see table)
  bit 4:    CM (Continuous Mail)
  bit 5:    MO (Mail Only)
  bit 6:    XA (Extended Addressing)
  bit 7:    XX (No requests)

If has extended flags (bit 5 = 1):
  - Extended flag bitfield (see below)

If has internet config (bit 4 = 1):
  - Internet config block (see below)

If has scheduling (bit 6 = 1):
  - Schedule block (see below)
```

### Node Type Encoding (3 bits)

```
Value  Type      Context Change
-----  --------  --------------
0      Zone      Sets new zone, resets net
1      Region    Sets region, may set net
2      Host      Sets new net
3      Hub       Hub under current net
4      Node      Regular node (most common)
5      Pvt       Private node
6      Down      Down node
7      Hold      Hold node
```

### Baud Rate Index (4 bits)

```
Index  Speed
-----  ------
0      300
1      1200
2      2400
3      9600
4      14400
5      19200
6      28800
7      33600
8      56000
9      57600
10     64000
11     115200
12     230400
13     reserved
14     reserved
15     custom (followed by varint)
```

### Extended Flag Bitfield (16 bits)

```
Bit   Flag    Description
---   ----    -----------
0     V32     V.32 modem
1     V32B    V.32bis
2     V34     V.34
3     VFC     V.Fast Class
4     V42B    V.42bis
5     V90C    V.90 client
6     V90S    V.90 server
7     H14     HST 14.4
8     H16     HST 16.8
9     HST     HST general
10    X75     X.75
11    ZYX     Zyxel
12    X2C     x2 client
13    X2S     x2 server
14    MN      No compression
15    ENC     Encrypted
```

If more flags needed, set bit 15 and follow with additional 16-bit word.

### Internet Config Block

Compact encoding for internet protocols:

```
Byte 0: Protocol presence bitfield
  bit 0: IBN (BinkP)
  bit 1: IFC (IFCICO)
  bit 2: ITN (Telnet)
  bit 3: IVM (VModem)
  bit 4: IFT (FTP)
  bit 5: IEM (Email)
  bit 6: ICM (Internet CM)
  bit 7: Has INA (hostname)

If has INA (bit 7 = 1):
  - Hostname string index (2 bytes)

For each protocol bit set (0-5):
  If port is non-default:
    - Port (2 bytes)
  Else:
    - Nothing (use default port)

Default ports:
  IBN: 24554
  IFC: 60179
  ITN: 23
  IVM: 3141
  IFT: 21
  IEM: (email address, use string index)
```

#### Optimized Protocol Encoding

For the common case of default ports:

```
Byte 0: Protocol bitfield (as above)

Byte 1 (if any protocol has non-default config):
  bits 0-5: Non-default port flags for protocols 0-5
  bit 6:    Has secondary hostname
  bit 7:    Has IPv6-only flag (INO4)

For each non-default port flag set:
  - Port number (2 bytes, little-endian)

If has INA:
  - Hostname index (2 bytes)

If has secondary hostname:
  - Secondary hostname index (2 bytes)

If has email (IEM):
  - Email string index (2 bytes)
```

### Schedule Block

For nodes with non-24/7 availability:

```
Byte 0: Schedule type
  0x00: 24/7 (no additional data)
  0x01: Daily schedule (DA flag)
  0x02: Weekday schedule (WK flag)
  0x03: Weekend schedule (WE flag)
  0x04: T-flag style (TAB, TCD, etc.)
  0x05: Custom (string index follows)

For types 0x01-0x03:
  - Start time (1 byte: hours * 2 + (minutes >= 30 ? 1 : 0))
  - End time (1 byte: same encoding)

For type 0x04 (T-flag):
  - Start letter (1 byte: 'A'-'X' mapped to 0-23)
  - End letter (1 byte)

For type 0x05:
  - Schedule string index (2 bytes)
```

## Varint Encoding

Standard protobuf-style variable-length integers:

```
Value Range        Bytes
-----------        -----
0-127             1 byte (MSB = 0)
128-16383         2 bytes (MSB = 1, continue)
16384-2097151     3 bytes
...               ...
```

Encoding:
```
while value >= 0x80:
    output(value & 0x7F | 0x80)
    value >>= 7
output(value)
```

## Delta Encoding

Within each net (Host block), node numbers are delta-encoded:

```
Host,100,NetCoord,...    -> base node = 0, store 0 (first in net)
,1,FirstNode,...         -> delta = 1-0 = 1, store varint(1)
,2,SecondNode,...        -> delta = 2-1 = 1, store varint(1)
,5,FifthNode,...         -> delta = 5-2 = 3, store varint(3)
,100,HubNode,...         -> delta = 100-5 = 95, store varint(95)
```

This works because nodes are typically sequential or nearly sequential.

## Duplicate/Conflict Handling

When HasConflict bit is set:
- Next byte is conflict sequence number (0-255)
- All fields follow normally
- Decoder should mark node with conflict flag

## Example Encoding

Source line:
```
Pvt,499,murphys.se,Uppsala,Peter_Laur,-Unpublished-,300,CM,XX,IFT,ITN,ITN:24,IFC:199,IBN:200
```

Binary encoding (hex):
```
45                    # Type=Pvt(5), no phone, has inet, no ext flags
F3 03                 # Delta node 499 (varint)
00 12                 # System name index 18 (murphys.se)
00 05                 # Location index 5 (Uppsala)
00 47                 # Sysop index 71 (Peter_Laur)
80                    # Baud=300(0), CM=1, MO=0, XA=0, XX=0
                      # Wait, XX is set, so:
90                    # Baud=300(0), CM=1, MO=0, XA=0, XX=1

# Internet config:
9F                    # IBN+IFC+ITN+IVM+IFT + has_INA
3F                    # Non-default ports: IBN, IFC, ITN (bits 0,1,2)
C8 00                 # IBN port 200
C7 00                 # IFC port 199
18 00                 # ITN port 24
00 12                 # INA: hostname index 18 (murphys.se)
```

Total: ~20 bytes vs ~80+ bytes in text = 4x compression before entropy coding

## Implementation Notes

### Encoder Algorithm

1. **First pass**: Build string dictionary
   - Collect all unique strings
   - Sort by category, then frequency
   - Assign indices

2. **Second pass**: Encode nodes
   - Track current zone/net/region context
   - Delta-encode node numbers within nets
   - Output binary records

3. **Compress**: Apply zstd/lzma to both blocks

4. **Finalize**: Write header, checksums

### Decoder Algorithm

1. Read and verify header
2. Decompress string dictionary, build lookup table
3. Decompress node data
4. Process records:
   - Maintain zone/net/region context
   - Reconstruct absolute node numbers from deltas
   - Lookup strings by index

### Memory Efficiency

For constrained environments:
- String dictionary can be memory-mapped
- Node records can be streamed (no random access needed)
- Per-net processing possible (flush after each net)

## Reference Implementation

### Compression Command (using standard tools)

```bash
# Step 1: Convert to binary (requires cnl-encoder tool)
cnl-encode nodelist.002 > nodelist.002.cnl-raw

# Step 2: Compress with zstd (level 19, long range matching)
zstd -19 --long=27 nodelist.002.cnl-raw -o nodelist.002.cnl

# With pre-trained dictionary (best compression):
zstd -19 --long=27 -D nodelist.dict nodelist.002.cnl-raw -o nodelist.002.cnl
```

### Decompression

```bash
# Decompress and decode
zstd -d -D nodelist.dict nodelist.002.cnl -o nodelist.002.cnl-raw
cnl-decode nodelist.002.cnl-raw > nodelist.002.txt
```

## Appendix A: Complete Flag Registry

### Boolean Flags (Bitfield Candidates)

```
Category: Modem (32 flags - 32 bits)
  V21, V22, V23, V29, V32, V32B, V32T, V33, V34, V42, V42B
  V90C, V90S, X2C, X2S, Z19, X75, HST, H96, H14, H16
  MAX, PEP, CSP, ZYX, VFC, MNP

Category: Capability (16 flags - 16 bits)
  CM, MO, LO, XA, XB, XC, XP, XR, XW, XX, MN

Category: User (8 flags - 8 bits)
  ENC, NC, NEC, REC, ZEC, PING, TRACE, RPK, RE

Category: Internet (8 flags - 8 bits)
  ICM, INO4, (6 reserved)
```

### Parameterized Flags (Require Value Storage)

```
Internet:
  IBN:port, IFC:port, ITN:port, IVM:port, IFT:port
  INA:hostname, IP:address
  IEM:email, IMI:email, ITX:email, IUC:email, ISE:email

Schedule:
  U:value, T:XX, DA:range, WK:range, WE:range, SU:range, SA:range

Zone Mail Hours:
  #01, #02, #08, #09, #18, #20
```

## Appendix B: Dictionary Training

For optimal compression, train a zstd dictionary on representative data:

```bash
# Collect sample nodelists (5-10 from different years)
mkdir samples
cp nodelist.001 nodelist.100 nodelist.200 nodelist.300 samples/

# Train dictionary (32KB is good tradeoff)
zstd --train -r samples/ -o nodelist.dict --maxdict=32768

# Use dictionary for compression
zstd -19 -D nodelist.dict input.cnl-raw -o output.cnl
```

Dictionary typically provides 15-25% better compression for nodelists.

## Appendix C: Differential Updates (Future Extension)

For incremental updates (nodediff equivalent):

```
+--------------------------------------------------+
| MAGIC (4 bytes): "CND\x01" (Compact Nodelist Diff)|
+--------------------------------------------------+
| Base nodelist day number (2 bytes)               |
| Target nodelist day number (2 bytes)             |
+--------------------------------------------------+
| Operations (compressed):                         |
|   ADD: type(1) + node record                     |
|   DEL: type(1) + zone:net/node (varints)         |
|   MOD: type(1) + zone:net/node + changed fields  |
+--------------------------------------------------+
```

Differential updates would be ~5-10% of full nodelist size.

## Version History

- v1.0 (2024): Initial specification

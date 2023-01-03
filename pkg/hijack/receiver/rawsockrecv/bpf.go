package rawsockrecv

import "golang.org/x/net/bpf"

func generateInstructions(port int) []bpf.Instruction {
	var instructions = []bpf.Instruction{
		bpf.LoadAbsolute{
			Off:  12,
			Size: 2,
		},
		bpf.JumpIf{ // IPv6
			Cond:      bpf.JumpEqual,
			Val:       0x86dd,
			SkipTrue:  0,
			SkipFalse: 4,
		},

		bpf.LoadAbsolute{ // IPv6 protocol header
			Off:  20,
			Size: 1,
		},
		bpf.JumpIf{ // if UDP
			Cond:      bpf.JumpEqual,
			Val:       0x11,
			SkipTrue:  0,
			SkipFalse: 18,
		},
		bpf.LoadConstant{
			Dst: bpf.RegX,
			Val: 14 + 40,
		},
		bpf.Jump{
			Skip: 9,
		},

		bpf.JumpIf{ // IPv4
			Cond:      bpf.JumpEqual,
			Val:       0x800,
			SkipTrue:  0,
			SkipFalse: 15,
		},

		bpf.LoadAbsolute{ // IPv4 protocol header
			Off:  23,
			Size: 1,
		},
		bpf.JumpIf{ // if UDP
			Cond:      bpf.JumpEqual,
			Val:       0x11,
			SkipTrue:  0,
			SkipFalse: 13,
		},

		bpf.LoadAbsolute{ // fragmented?
			Off:  20,
			Size: 2,
		},
		bpf.JumpIf{
			Cond:      bpf.JumpBitsSet,
			Val:       0x1fff,
			SkipTrue:  11,
			SkipFalse: 0,
		},

		bpf.LoadMemShift{
			Off: 14,
		},
		bpf.LoadConstant{
			Dst: bpf.RegA,
			Val: 14,
		},
		bpf.ALUOpX{
			Op: bpf.ALUOpAdd,
		},
		bpf.TAX{}, // X = 4*([14]&0xf) + 14

		bpf.LoadIndirect{ // UDP dst port
			Off:  2,
			Size: 2,
		},
		bpf.JumpIf{
			Cond:      bpf.JumpEqual,
			Val:       uint32(port),
			SkipTrue:  0,
			SkipFalse: 5,
		},

		bpf.LoadIndirect{ // first byte in payload
			Off:  8,
			Size: 1,
		},
		bpf.JumpIf{ // payload[0] & 128 != 0
			Cond:      bpf.JumpBitsSet,
			Val:       128,
			SkipTrue:  2,
			SkipFalse: 0,
		},

		bpf.LoadIndirect{
			Off:  12,
			Size: 4,
		},
		bpf.JumpIf{ // payload[4:8] == 0x2112a442
			Cond:      bpf.JumpEqual,
			Val:       0x2112a442,
			SkipTrue:  0,
			SkipFalse: 1,
		},

		bpf.RetConstant{
			Val: 1 << 18,
		},
		bpf.RetConstant{
			Val: 0,
		},
	}

	return instructions
}

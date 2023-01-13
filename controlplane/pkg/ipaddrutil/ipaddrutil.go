package ipaddrutil

import (
	"github.com/seancfoley/ipaddress-go/ipaddr"
)

// FreeBlocks finds free unused ranges from base excluding used
func FreeBlocks(base *ipaddr.IPAddress, used []*ipaddr.IPAddress) []*ipaddr.IPAddress {
	freeBlocks := []*ipaddr.IPAddress{
		base,
	}

	for _, u := range used {
		subtracted := []*ipaddr.IPAddress{}

		for _, block := range freeBlocks {
			subtracted = append(subtracted, block.Subtract(u)...)
		}

		freeBlocks = subtracted
	}

	if len(freeBlocks) == 0 {
		return nil
	}

	spanned := []*ipaddr.IPAddress{}
	for _, block := range freeBlocks {
		spanned = append(spanned, block.SpanWithPrefixBlocks()...)
	}

	return spanned[0].MergeToPrefixBlocks(spanned[1:]...)
}

// FindSubBlock finds a block with 2^sizeBit addresses from blocks
func FindSubBlock(blocks []*ipaddr.IPAddress, sizeBit int) *ipaddr.IPAddress {
	for _, block := range blocks {
		blockSizeBit := block.GetBitCount() - int(*block.GetPrefixLen())

		if blockSizeBit < sizeBit {
			continue
		}

		iter := block.SetPrefixLen(block.GetBitCount() - sizeBit).PrefixBlockIterator()

		return iter.Next()
	}

	return nil
}

package brew

import (
	"sync"

	"github.com/klauspost/compress/zstd"
)

var (
	encoderPool sync.Pool
	decoderPool sync.Pool
)

func getEncoder() *zstd.Encoder {
	if v := encoderPool.Get(); v != nil {
		return v.(*zstd.Encoder)
	}
	enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	return enc
}

func putEncoder(enc *zstd.Encoder) {
	encoderPool.Put(enc)
}

func getDecoder() *zstd.Decoder {
	if v := decoderPool.Get(); v != nil {
		return v.(*zstd.Decoder)
	}
	dec, _ := zstd.NewReader(nil, zstd.IgnoreChecksum(true))
	return dec
}

func putDecoder(dec *zstd.Decoder) {
	decoderPool.Put(dec)
}

func compressWithPool(data []byte) []byte {
	enc := getEncoder()
	defer putEncoder(enc)
	return enc.EncodeAll(data, nil)
}

func decompressWithPool(data []byte) ([]byte, error) {
	dec := getDecoder()
	defer putDecoder(dec)
	return dec.DecodeAll(data, nil)
}

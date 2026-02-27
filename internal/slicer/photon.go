package slicer

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
)

const (
	photonMagic   = 0x12fd0086
	photonVersion = 2
)

// PhotonHeader contains the file header information.
type PhotonHeader struct {
	BedXMM          float32
	BedYMM          float32
	BedZMM          float32
	LayerHeightMM   float32
	ExposureS       float32
	BottomExposureS float32
	BottomLayers    uint32
	ResolutionX     uint32
	ResolutionY     uint32
	LayerCount      uint32
	LiftHeightMM    float32
	LiftSpeedMMPS   float32
	RetractSpeedMMPS float32
	AntiAliasing    uint32
}

// RLEEncode encodes a grayscale image using the Photon RLE format.
// Scans column by column (X varies fastest in photon format).
// Bit 7 = color (0=dark, 1=light), bits 0-6 = run length (max 125).
func RLEEncode(img *image.Gray) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	var encoded []byte
	var currentColor byte
	var runLength int

	flush := func() {
		if runLength == 0 {
			return
		}
		for runLength > 0 {
			chunk := runLength
			if chunk > 125 {
				chunk = 125
			}
			b := byte(chunk)
			if currentColor > 0 {
				b |= 0x80
			}
			encoded = append(encoded, b)
			runLength -= chunk
		}
	}

	// Photon format scans column-by-column
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := img.GrayAt(x, y).Y
			var c byte
			if pixel >= 128 {
				c = 1
			}

			if runLength == 0 {
				currentColor = c
				runLength = 1
			} else if c == currentColor {
				runLength++
			} else {
				flush()
				currentColor = c
				runLength = 1
			}
		}
	}
	flush()

	return encoded
}

// WritePhotonFile writes a complete .photon file.
func WritePhotonFile(w io.Writer, header PhotonHeader, layers [][]byte) error {
	// Calculate offsets
	headerSize := uint32(76)
	previewLargeSize := uint32(0) // Skip preview for now
	previewSmallSize := uint32(0)

	layerTableOffset := headerSize + previewLargeSize + previewSmallSize
	layerEntrySize := uint32(36) // each layer table entry
	layerTableSize := layerEntrySize * header.LayerCount

	// Calculate layer data offsets
	layerDataOffset := layerTableOffset + layerTableSize
	layerOffsets := make([]uint32, header.LayerCount)
	currentOffset := layerDataOffset
	for i := uint32(0); i < header.LayerCount; i++ {
		layerOffsets[i] = currentOffset
		currentOffset += uint32(len(layers[i]))
	}

	// Write header (76 bytes)
	if err := writeHeader(w, header, layerTableOffset, previewLargeSize, previewSmallSize); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write layer table
	for i := uint32(0); i < header.LayerCount; i++ {
		layerZ := float32(i+1) * header.LayerHeightMM
		exposure := header.ExposureS
		if i < header.BottomLayers {
			exposure = header.BottomExposureS
		}

		entry := struct {
			LayerZ     float32
			DataOffset uint32
			DataLength uint32
			Exposure   float32
			LiftHeight float32
			LiftSpeed  float32
			Padding    [12]byte
		}{
			LayerZ:     layerZ,
			DataOffset: layerOffsets[i],
			DataLength: uint32(len(layers[i])),
			Exposure:   exposure,
			LiftHeight: header.LiftHeightMM,
			LiftSpeed:  header.LiftSpeedMMPS,
		}

		if err := binary.Write(w, binary.LittleEndian, &entry); err != nil {
			return fmt.Errorf("write layer table entry %d: %w", i, err)
		}
	}

	// Write layer data
	for i := uint32(0); i < header.LayerCount; i++ {
		if _, err := w.Write(layers[i]); err != nil {
			return fmt.Errorf("write layer data %d: %w", i, err)
		}
	}

	return nil
}

func writeHeader(w io.Writer, h PhotonHeader, layerTableOffset, previewLargeOff, previewSmallOff uint32) error {
	// The photon header is 76 bytes
	buf := make([]byte, 76)

	binary.LittleEndian.PutUint32(buf[0:4], photonMagic)
	binary.LittleEndian.PutUint32(buf[4:8], photonVersion)

	putFloat32 := func(offset int, v float32) {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], math.Float32bits(v))
	}

	putFloat32(8, h.BedXMM)
	putFloat32(12, h.BedYMM)
	putFloat32(16, h.BedZMM)

	// Padding 3 * 4 bytes (offsets 20-31)
	// offset 20: padding/unused
	// offset 24: padding/unused
	// offset 28: padding/unused

	putFloat32(32, h.LayerHeightMM)
	putFloat32(36, h.ExposureS)
	putFloat32(40, h.BottomExposureS)

	binary.LittleEndian.PutUint32(buf[44:48], h.BottomLayers)
	binary.LittleEndian.PutUint32(buf[48:52], h.ResolutionX)
	binary.LittleEndian.PutUint32(buf[52:56], h.ResolutionY)

	// Preview offsets (0 = no preview)
	binary.LittleEndian.PutUint32(buf[56:60], 0) // preview large offset
	binary.LittleEndian.PutUint32(buf[60:64], layerTableOffset)

	binary.LittleEndian.PutUint32(buf[64:68], h.LayerCount)

	// Preview small offset
	binary.LittleEndian.PutUint32(buf[68:72], 0) // preview small offset

	binary.LittleEndian.PutUint32(buf[72:76], h.AntiAliasing)

	_, err := w.Write(buf)
	return err
}

// GeneratePreview creates a simple top-down preview image from the first layer.
func GeneratePreview(img *image.Gray, targetWidth, targetHeight int) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	preview := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	scaleX := float64(srcW) / float64(targetWidth)
	scaleY := float64(srcH) / float64(targetHeight)

	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			sx := int(float64(x) * scaleX)
			sy := int(float64(y) * scaleY)
			if sx >= srcW {
				sx = srcW - 1
			}
			if sy >= srcH {
				sy = srcH - 1
			}

			gray := img.GrayAt(sx, sy).Y
			if gray > 0 {
				preview.Set(x, y, color.RGBA{R: 100, G: 80, B: 200, A: 255})
			} else {
				preview.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
	}

	return preview
}

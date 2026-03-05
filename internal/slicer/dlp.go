package slicer

import (
	"encoding/binary"
	"image"
	"io"
	"math"
	"3dmodels/internal/models"
)

// Anycubic DLP (.dlp) Binary Format Constants
const (
	anycubicMagic = "ANYCUBIC"
	headerMagic   = "HEADER"
	previewMagic  = "PREVIEW"
	layerdefMagic = "LAYERDEF"
)

// WriteDLPFile writes an Anycubic binary .dlp file.
func WriteDLPFile(w io.Writer, profile *models.PrinterProfile, settings *models.PrintSettings, layers []*image.Gray) error {
	// 1. Calculate Offsets
	// Main Header: 72 bytes
	// HEADER section: 8 (magic) + 80 (data) = 88 bytes
	// PREVIEW section: 8 (magic) + 12 (info) + (224*168*2) = 75284 bytes (using RGB565)
	// LAYERDEF section: 8 (magic) + 8 (info) + (numLayers * 32) = 16 + numLayers*32 bytes
	
	headerOffset := uint32(72)
	previewOffset := headerOffset + 88
	layerdefOffset := previewOffset + 75284
	
	// Encode layers first to know their sizes and offsets
	encodedLayers := make([][]byte, len(layers))
	totalDataSize := uint32(0)
	for i, img := range layers {
		encodedLayers[i] = RLEEncode(img) // Reuse Photon RLE for now, Anycubic often uses similar
		totalDataSize += uint32(len(encodedLayers[i]))
	}
	
	layerDataStart := layerdefOffset + 16 + uint32(len(layers))*32

	// 2. Write Main Header (72 bytes)
	mainHeader := make([]byte, 72)
	copy(mainHeader[0:8], anycubicMagic)
	binary.LittleEndian.PutUint32(mainHeader[8:12], 1) // Version
	binary.LittleEndian.PutUint32(mainHeader[12:16], headerOffset)
	binary.LittleEndian.PutUint32(mainHeader[16:20], 0) // Padding
	binary.LittleEndian.PutUint32(mainHeader[20:24], previewOffset)
	binary.LittleEndian.PutUint32(mainHeader[24:28], 0) // Padding
	binary.LittleEndian.PutUint32(mainHeader[28:32], layerdefOffset)
	// Remaining bytes are padding/reserved
	if _, err := w.Write(mainHeader); err != nil {
		return err
	}

	// 3. Write HEADER section (88 bytes)
	hSec := make([]byte, 88)
	copy(hSec[0:8], headerMagic)
	// Header data starts at offset 8
	binary.LittleEndian.PutUint32(hSec[8:12], 80) // Header data length
	putFloat32(hSec[12:16], float32(settings.LayerHeightMM))
	putFloat32(hSec[16:20], float32(settings.ExposureTimeS))
	putFloat32(hSec[20:24], float32(settings.BottomExposureS))
	putFloat32(hSec[24:28], 0.5) // Off time
	binary.LittleEndian.PutUint32(hSec[28:32], uint32(settings.BottomLayers))
	putFloat32(hSec[32:36], float32(profile.BuildWidthMM))
	putFloat32(hSec[36:40], float32(profile.BuildDepthMM))
	putFloat32(hSec[40:44], float32(profile.BuildHeightMM))
	// More params... (simplified)
	binary.LittleEndian.PutUint32(hSec[44:48], uint32(profile.ResolutionX))
	binary.LittleEndian.PutUint32(hSec[48:52], uint32(profile.ResolutionY))
	if _, err := w.Write(hSec); err != nil {
		return err
	}

	// 4. Write PREVIEW section (75284 bytes)
	pSec := make([]byte, 75284)
	copy(pSec[0:8], previewMagic)
	binary.LittleEndian.PutUint32(pSec[8:12], 75276) // Preview data length
	binary.LittleEndian.PutUint32(pSec[12:16], 224) // Width
	binary.LittleEndian.PutUint32(pSec[16:20], 168) // Height
	// Fill preview with black (0x0000 in RGB565) or a simple pattern
	// ... (pixels start at 20)
	if _, err := w.Write(pSec); err != nil {
		return err
	}

	// 5. Write LAYERDEF section
	lDefHeader := make([]byte, 16)
	copy(lDefHeader[0:8], layerdefMagic)
	binary.LittleEndian.PutUint32(lDefHeader[8:12], uint32(len(layers))*32 + 4)
	binary.LittleEndian.PutUint32(lDefHeader[12:16], uint32(len(layers)))
	if _, err := w.Write(lDefHeader); err != nil {
		return err
	}

	currentOffset := layerDataStart
	for i := 0; i < len(layers); i++ {
		lEntry := make([]byte, 32)
		binary.LittleEndian.PutUint32(lEntry[0:4], currentOffset)
		binary.LittleEndian.PutUint32(lEntry[4:8], uint32(len(encodedLayers[i])))
		putFloat32(lEntry[8:12], float32(float64(i+1)*settings.LayerHeightMM))
		putFloat32(lEntry[12:16], float32(settings.ExposureTimeS))
		if i < settings.BottomLayers {
			putFloat32(lEntry[12:16], float32(settings.BottomExposureS))
		}
		// Padding 16 bytes
		if _, err := w.Write(lEntry); err != nil {
			return err
		}
		currentOffset += uint32(len(encodedLayers[i]))
	}

	// 6. Write Layer Data
	for _, data := range encodedLayers {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}

	return nil
}

func putFloat32(b []byte, v float32) {
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}

package slicer

import (
	"archive/zip"
	"fmt"
	"image"
	"image/png"
	"io"
	"3dmodels/internal/models"
)

// WriteDLPFile writes a .dlp file which is a zip containing a config and PNG layers.
// Note: This is a simplified version of the DLP format.
func WriteDLPFile(w io.Writer, profile *models.PrinterProfile, settings *models.PrintSettings, layers []*image.Gray) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	// 1. Write config file
	cf, err := zw.Create("run.txt")
	if err != nil {
		return err
	}
	configStr := fmt.Sprintf(`property {
  name: %s
  width: %d
  height: %d
  pixel_size: %.4f
  layers: %d
  layer_height: %.4f
  exposure_time: %.2f
  bottom_exposure_time: %.2f
  bottom_layers: %d
}
`,
		profile.Name, profile.ResolutionX, profile.ResolutionY, profile.PixelSizeUM/1000.0,
		len(layers), settings.LayerHeightMM, settings.ExposureTimeS,
		settings.BottomExposureS, settings.BottomLayers,
	)
	if _, err := cf.Write([]byte(configStr)); err != nil {
		return err
	}

	// 2. Write layers as PNGs
	for i, img := range layers {
		f, err := zw.Create(fmt.Sprintf("%04d.png", i))
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			return err
		}
	}

	return nil
}

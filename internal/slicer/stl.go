package slicer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
)

type Triangle struct {
	Normal    [3]float32
	V1, V2, V3 [3]float32
}

type Mesh struct {
	Triangles []Triangle
	MinBound  [3]float32
	MaxBound  [3]float32
}

func ParseSTL(filePath string) (*Mesh, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open STL: %w", err)
	}
	defer f.Close()

	// Read first 80 bytes (header) + 4 bytes (face count) to detect format
	header := make([]byte, 84)
	n, err := f.Read(header)
	if err != nil || n < 84 {
		// File too small, try ASCII
		f.Seek(0, 0)
		return parseSTLASCII(f)
	}

	// Check if file starts with "solid" - ASCII STL files always start with this keyword
	headerStr := strings.TrimSpace(string(header[:min(80, n)]))
	if strings.HasPrefix(headerStr, "solid") {
		log.Printf("STL %s: Detected as ASCII STL (header starts with 'solid')", filePath)
		f.Seek(0, 0)
		return parseSTLASCII(f)
	}

	// For binary, verify the face count makes sense
	faceCount := binary.LittleEndian.Uint32(header[80:84])
	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	
	expectedSize := int64(84) + int64(faceCount)*50
	
	// If file size doesn't match binary format exactly, try ASCII
	if expectedSize != stat.Size() {
		log.Printf("STL %s: Binary size mismatch (expected %d, got %d), parsing as ASCII", filePath, expectedSize, stat.Size())
		f.Seek(0, 0)
		return parseSTLASCII(f)
	}

	// Try parsing as binary first
	log.Printf("STL %s: Attempting binary parse (%d faces)", filePath, faceCount)
	f.Seek(80, 0)
	mesh, err := parseSTLBinary(f)
	
	// If binary parsing succeeded but produced invalid bounds, re-parse as ASCII
	if err == nil && mesh != nil {
		height := float64(mesh.MaxBound[2] - mesh.MinBound[2])
		// If height is NaN or Inf, or unreasonably large, try re-parsing as ASCII
		// But don't re-parse if height is small (flat model) if it's clearly binary by size
		if math.IsNaN(height) || math.IsInf(height, 0) || height > 1000000 {
			log.Printf("STL %s: Binary parse produced invalid bounds (height=%.4f), re-parsing as ASCII", filePath, height)
			f.Seek(0, 0)
			return parseSTLASCII(f)
		}
		
		// If binary parse successful, check bounds height for logging
		log.Printf("STL %s: Binary parse successful, bounds height=%.2fmm", filePath, height)
	}
	
	return mesh, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseSTLBinary(f *os.File) (*Mesh, error) {
	var faceCount uint32
	if err := binary.Read(f, binary.LittleEndian, &faceCount); err != nil {
		return nil, fmt.Errorf("read face count: %w", err)
	}

	if faceCount == 0 || faceCount > 50_000_000 {
		return nil, fmt.Errorf("invalid face count: %d", faceCount)
	}

	mesh := &Mesh{
		Triangles: make([]Triangle, 0, faceCount),
		MinBound:  [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		MaxBound:  [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}

	buf := bufio.NewReader(f)
	faceBuf := make([]byte, 50)
	invalidFaces := 0

	for i := uint32(0); i < faceCount; i++ {
		if _, err := buf.Read(faceBuf); err != nil {
			return nil, fmt.Errorf("read face %d: %w", i, err)
		}

		var tri Triangle
		tri.Normal[0] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[0:4]))
		tri.Normal[1] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[4:8]))
		tri.Normal[2] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[8:12]))

		// If normal is invalid, just zero it out instead of skipping the face.
		// Slicer doesn't use normals, and many STLs have garbage normals.
		if !isValidFloat(tri.Normal[0]) || !isValidFloat(tri.Normal[1]) || !isValidFloat(tri.Normal[2]) {
			tri.Normal = [3]float32{0, 0, 0}
		}

		validFace := true
		for v := 0; v < 3; v++ {
			off := 12 + v*12
			x := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off : off+4]))
			y := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off+4 : off+8]))
			z := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off+8 : off+12]))

			// Skip faces with invalid coordinates (NaN or Inf) or coordinates out of reasonable range
			if !isValidFloat(x) || !isValidFloat(y) || !isValidFloat(z) ||
				math.Abs(float64(x)) > 100000 || math.Abs(float64(y)) > 100000 || math.Abs(float64(z)) > 100000 {
				validFace = false
				break
			}

			vert := [3]float32{x, y, z}
			switch v {
			case 0:
				tri.V1 = vert
			case 1:
				tri.V2 = vert
			case 2:
				tri.V3 = vert
			}
		}

		if validFace {
			updateBounds(mesh, tri.V1[0], tri.V1[1], tri.V1[2])
			updateBounds(mesh, tri.V2[0], tri.V2[1], tri.V2[2])
			updateBounds(mesh, tri.V3[0], tri.V3[1], tri.V3[2])
			mesh.Triangles = append(mesh.Triangles, tri)
		} else {
			invalidFaces++
		}
	}

	if len(mesh.Triangles) == 0 {
		return nil, fmt.Errorf("no valid triangles found in STL file")
	}

	if invalidFaces > 0 {
		log.Printf("Warning: Repaired STL by removing %d invalid faces (kept %d triangles)", invalidFaces, len(mesh.Triangles))
	}

	return mesh, nil
}

func parseSTLASCII(f *os.File) (*Mesh, error) {
	mesh := &Mesh{
		MinBound: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		MaxBound: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var tri Triangle
	vertIdx := 0
	invalidFaces := 0
	validFaces := 0
	lineCount := 0
	solidFound := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineCount++

		// Skip empty lines
		if line == "" {
			continue
		}

		// Verify this looks like ASCII STL
		if !solidFound && strings.HasPrefix(line, "solid") {
			solidFound = true
			continue
		}

		if strings.HasPrefix(line, "facet normal") {
			// Parse normal with better error handling
			var nx, ny, nz float64
			n, err := fmt.Sscanf(line, "facet normal %f %f %f", &nx, &ny, &nz)
			if err != nil || n != 3 {
				continue // Skip malformed lines
			}
			tri.Normal[0] = float32(nx)
			tri.Normal[1] = float32(ny)
			tri.Normal[2] = float32(nz)
			
			// Validate normal
			if !isValidFloat(tri.Normal[0]) || !isValidFloat(tri.Normal[1]) || !isValidFloat(tri.Normal[2]) {
				continue
			}
			vertIdx = 0
		} else if strings.HasPrefix(line, "vertex") {
			// Parse vertex with better error handling
			var vx, vy, vz float64
			n, err := fmt.Sscanf(line, "vertex %f %f %f", &vx, &vy, &vz)
			if err != nil || n != 3 {
				vertIdx = -1 // Mark face as invalid
				continue
			}
			
			x, y, z := float32(vx), float32(vy), float32(vz)

			// Check if vertex is valid and within reasonable bounds
			if !isValidFloat(x) || !isValidFloat(y) || !isValidFloat(z) {
				vertIdx = -1 // Mark this face as invalid
				continue
			}
			
			// Additional sanity check: coordinates should be within reasonable range
			// (e.g., -10000 to +10000 mm for typical 3D prints)
			if math.Abs(float64(x)) > 100000 || math.Abs(float64(y)) > 100000 || math.Abs(float64(z)) > 100000 {
				log.Printf("Warning: Vertex coordinates out of reasonable range: (%.2f, %.2f, %.2f) at line %d", x, y, z, lineCount)
				vertIdx = -1
				continue
			}

			if vertIdx >= 0 {
				switch vertIdx {
				case 0:
					tri.V1 = [3]float32{x, y, z}
				case 1:
					tri.V2 = [3]float32{x, y, z}
				case 2:
					tri.V3 = [3]float32{x, y, z}
				}
				updateBounds(mesh, x, y, z)
			}
			vertIdx++
		} else if strings.HasPrefix(line, "endfacet") {
			if vertIdx >= 3 {
				mesh.Triangles = append(mesh.Triangles, tri)
				validFaces++
			} else if vertIdx >= 0 {
				invalidFaces++
			}
			tri = Triangle{}
			vertIdx = 0
		}
	}

	if len(mesh.Triangles) == 0 {
		return nil, fmt.Errorf("no valid triangles found in ASCII STL")
	}

	log.Printf("ASCII STL parsed: %d lines, %d valid faces, %d invalid faces", lineCount, validFaces, invalidFaces)
	if invalidFaces > 0 {
		log.Printf("Warning: Repaired ASCII STL by removing %d invalid faces (kept %d triangles)", invalidFaces, validFaces)
	}

	log.Printf("STL bounds: [%.4f,%.4f,%.4f] to [%.4f,%.4f,%.4f], height=%.2fmm, triangles=%d",
		mesh.MinBound[0], mesh.MinBound[1], mesh.MinBound[2],
		mesh.MaxBound[0], mesh.MaxBound[1], mesh.MaxBound[2],
		float64(mesh.MaxBound[2])-float64(mesh.MinBound[2]),
		len(mesh.Triangles))

	return mesh, nil
}

func updateBounds(mesh *Mesh, x, y, z float32) {
	if x < mesh.MinBound[0] {
		mesh.MinBound[0] = x
	}
	if y < mesh.MinBound[1] {
		mesh.MinBound[1] = y
	}
	if z < mesh.MinBound[2] {
		mesh.MinBound[2] = z
	}
	if x > mesh.MaxBound[0] {
		mesh.MaxBound[0] = x
	}
	if y > mesh.MaxBound[1] {
		mesh.MaxBound[1] = y
	}
	if z > mesh.MaxBound[2] {
		mesh.MaxBound[2] = z
	}
}

// Scale multiplies all vertex coordinates and bounds by the given factor.
func (m *Mesh) Scale(factor float32) {
	for i := range m.Triangles {
		t := &m.Triangles[i]
		for _, v := range []*[3]float32{&t.V1, &t.V2, &t.V3} {
			v[0] *= factor
			v[1] *= factor
			v[2] *= factor
		}
	}
	for i := 0; i < 3; i++ {
		m.MinBound[i] *= factor
		m.MaxBound[i] *= factor
	}
	// Re-sort bounds if factor was negative (not expected here)
	if factor < 0 {
		for i := 0; i < 3; i++ {
			if m.MinBound[i] > m.MaxBound[i] {
				m.MinBound[i], m.MaxBound[i] = m.MaxBound[i], m.MinBound[i]
			}
		}
	}
}

// CenterOnPlate shifts the mesh so its center XY is at the given offset and bottom Z is at 0.
func (m *Mesh) CenterOnPlate(offsetX, offsetY float64) {
	cx := float32((float64(m.MinBound[0]) + float64(m.MaxBound[0])) / 2)
	cy := float32((float64(m.MinBound[1]) + float64(m.MaxBound[1])) / 2)
	zMin := m.MinBound[2]

	dx := float32(offsetX) - cx
	dy := float32(offsetY) - cy
	dz := -zMin

	for i := range m.Triangles {
		t := &m.Triangles[i]
		for _, v := range []*[3]float32{&t.V1, &t.V2, &t.V3} {
			v[0] += dx
			v[1] += dy
			v[2] += dz
		}
	}

	m.MinBound[0] += dx
	m.MaxBound[0] += dx
	m.MinBound[1] += dy
	m.MaxBound[1] += dy
	m.MinBound[2] += dz
	m.MaxBound[2] += dz
}

// MergeMesh appends all triangles from other into m and updates bounds.
func (m *Mesh) MergeMesh(other *Mesh) {
	m.Triangles = append(m.Triangles, other.Triangles...)
	for i := 0; i < 3; i++ {
		if other.MinBound[i] < m.MinBound[i] {
			m.MinBound[i] = other.MinBound[i]
		}
		if other.MaxBound[i] > m.MaxBound[i] {
			m.MaxBound[i] = other.MaxBound[i]
		}
	}
}

// isValidFloat checks if a float32 value is valid (not NaN or Infinite).
func isValidFloat(f float32) bool {
	return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}

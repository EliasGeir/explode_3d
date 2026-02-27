package slicer

import (
	"bufio"
	"encoding/binary"
	"fmt"
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
		// Try ASCII
		f.Seek(0, 0)
		return parseSTLASCII(f)
	}

	// Check if starts with "solid" and doesn't have binary-looking data
	if strings.HasPrefix(strings.TrimSpace(string(header[:5])), "solid") {
		// Could be ASCII - check if the face count makes sense for binary
		faceCount := binary.LittleEndian.Uint32(header[80:84])
		stat, _ := f.Stat()
		expectedSize := int64(84) + int64(faceCount)*50
		if stat != nil && expectedSize != stat.Size() {
			// Size mismatch → probably ASCII
			f.Seek(0, 0)
			return parseSTLASCII(f)
		}
	}

	// Parse as binary
	f.Seek(80, 0)
	return parseSTLBinary(f)
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

	for i := uint32(0); i < faceCount; i++ {
		if _, err := buf.Read(faceBuf); err != nil {
			return nil, fmt.Errorf("read face %d: %w", i, err)
		}

		var tri Triangle
		tri.Normal[0] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[0:4]))
		tri.Normal[1] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[4:8]))
		tri.Normal[2] = math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[8:12]))

		for v := 0; v < 3; v++ {
			off := 12 + v*12
			x := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off : off+4]))
			y := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off+4 : off+8]))
			z := math.Float32frombits(binary.LittleEndian.Uint32(faceBuf[off+8 : off+12]))

			vert := [3]float32{x, y, z}
			switch v {
			case 0:
				tri.V1 = vert
			case 1:
				tri.V2 = vert
			case 2:
				tri.V3 = vert
			}

			updateBounds(mesh, x, y, z)
		}
		// Skip 2 bytes attribute byte count
		mesh.Triangles = append(mesh.Triangles, tri)
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

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "facet normal") {
			fmt.Sscanf(line, "facet normal %f %f %f", &tri.Normal[0], &tri.Normal[1], &tri.Normal[2])
			vertIdx = 0
		} else if strings.HasPrefix(line, "vertex") {
			var x, y, z float32
			fmt.Sscanf(line, "vertex %f %f %f", &x, &y, &z)
			switch vertIdx {
			case 0:
				tri.V1 = [3]float32{x, y, z}
			case 1:
				tri.V2 = [3]float32{x, y, z}
			case 2:
				tri.V3 = [3]float32{x, y, z}
			}
			updateBounds(mesh, x, y, z)
			vertIdx++
		} else if strings.HasPrefix(line, "endfacet") {
			mesh.Triangles = append(mesh.Triangles, tri)
			tri = Triangle{}
		}
	}

	if len(mesh.Triangles) == 0 {
		return nil, fmt.Errorf("no triangles found in ASCII STL")
	}

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

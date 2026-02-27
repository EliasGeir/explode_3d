package slicer

import (
	"math"
	"sort"
)

type Point2D struct {
	X, Y float64
}

type Contour []Point2D

// SliceAtZ intersects the mesh with a horizontal plane at the given Z height
// and returns closed contours.
func SliceAtZ(mesh *Mesh, z float32) []Contour {
	var segments [][2]Point2D

	for i := range mesh.Triangles {
		tri := &mesh.Triangles[i]
		p1, p2, ok := intersectTrianglePlane(tri, z)
		if ok {
			segments = append(segments, [2]Point2D{p1, p2})
		}
	}

	if len(segments) == 0 {
		return nil
	}

	return linkSegments(segments)
}

// intersectTrianglePlane finds the intersection of a triangle with a Z plane.
// Returns two points (a line segment) if the plane intersects, ok=false otherwise.
func intersectTrianglePlane(tri *Triangle, z float32) (p1, p2 Point2D, ok bool) {
	verts := [3][3]float32{tri.V1, tri.V2, tri.V3}

	// Classify each vertex: +1 above, -1 below, 0 on plane
	var side [3]int
	for i, v := range verts {
		diff := v[2] - z
		if diff > 1e-6 {
			side[i] = 1
		} else if diff < -1e-6 {
			side[i] = -1
		} else {
			side[i] = 0
		}
	}

	// Collect intersection points
	var points [2]Point2D
	idx := 0

	// Check edges for crossings
	edges := [3][2]int{{0, 1}, {1, 2}, {2, 0}}
	for _, edge := range edges {
		a, b := edge[0], edge[1]
		sa, sb := side[a], side[b]

		if sa == 0 && sb == 0 {
			// Both on plane — coplanar edge, skip (would produce degenerate segment)
			continue
		}

		if sa == 0 {
			// Vertex a is on the plane
			if idx < 2 {
				points[idx] = Point2D{X: float64(verts[a][0]), Y: float64(verts[a][1])}
				idx++
			}
			continue
		}

		if sb == 0 {
			// Vertex b is on the plane — will be picked up when it's vertex a of another edge
			continue
		}

		if sa != sb {
			// Edge crosses the plane
			t := float64(z-verts[a][2]) / float64(verts[b][2]-verts[a][2])
			if idx < 2 {
				points[idx] = Point2D{
					X: float64(verts[a][0]) + t*(float64(verts[b][0])-float64(verts[a][0])),
					Y: float64(verts[a][1]) + t*(float64(verts[b][1])-float64(verts[a][1])),
				}
				idx++
			}
		}
	}

	if idx < 2 {
		return Point2D{}, Point2D{}, false
	}

	return points[0], points[1], true
}

const linkEpsilon = 0.001 // mm tolerance for linking segments

// linkSegments connects line segments into closed contours.
func linkSegments(segments [][2]Point2D) []Contour {
	if len(segments) == 0 {
		return nil
	}

	used := make([]bool, len(segments))
	var contours []Contour

	// Build a spatial index for segment endpoints
	type endpointInfo struct {
		segIdx int
		end    int // 0 = start, 1 = end
	}

	for {
		// Find first unused segment
		startIdx := -1
		for i, u := range used {
			if !u {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			break
		}

		used[startIdx] = true
		contour := Contour{segments[startIdx][0], segments[startIdx][1]}
		current := segments[startIdx][1]

		for {
			found := false
			for i, seg := range segments {
				if used[i] {
					continue
				}

				if pointsClose(seg[0], current) {
					used[i] = true
					contour = append(contour, seg[1])
					current = seg[1]
					found = true
					break
				}
				if pointsClose(seg[1], current) {
					used[i] = true
					contour = append(contour, seg[0])
					current = seg[0]
					found = true
					break
				}
			}
			if !found {
				break
			}
			// Check if contour is closed
			if pointsClose(contour[0], current) {
				break
			}
		}

		if len(contour) >= 3 {
			contours = append(contours, contour)
		}
	}

	return contours
}

func pointsClose(a, b Point2D) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return math.Abs(dx) < linkEpsilon && math.Abs(dy) < linkEpsilon
}

// SortContoursByArea sorts contours so outer contours come first.
func SortContoursByArea(contours []Contour) {
	sort.Slice(contours, func(i, j int) bool {
		return math.Abs(contourArea(contours[i])) > math.Abs(contourArea(contours[j]))
	})
}

func contourArea(c Contour) float64 {
	area := 0.0
	n := len(c)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		area += c[i].X*c[j].Y - c[j].X*c[i].Y
	}
	return area / 2
}

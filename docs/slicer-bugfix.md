# Slicer Bugfix - Memory Exhaustion & Concurrency

## Problem Description
Users reported that during slicing of 3D models, the process would occasionally fail with a "Job not found" (404) error during status polling.

Analysis of the HTTP Archive (HAR) revealed the following sequence:
1. Two slicing jobs were initiated almost simultaneously (likely due to multiple clicks on the "Start Slice" button).
2. The first job started correctly.
3. The second job was initiated.
4. Subsequent polling for the second job returned a connection closed error (status 0) followed by 404 Not Found for all future requests.

## Root Cause Analysis
The symptoms strongly suggest a **server crash and automatic restart**.

When the server crashes:
1. The in-memory map of slicing jobs is cleared.
2. Any ongoing goroutines (including the slicing workers) are terminated.
3. Subsequent status requests for previously valid job IDs return 404 because the IDs no longer exist in the fresh map.

The crash was likely caused by **Memory Exhaustion (OOM)**. Slicing 3D models is memory-intensive:
- Each STL file is parsed into a mesh of triangles (~48 bytes per triangle).
- Multiple meshes are merged, potentially doubling memory usage during `append` operations.
- Rasterization creates high-resolution grayscale images for each layer.
- Running multiple slicing jobs in parallel multiplies this memory usage, exceeding the available RAM on the host machine.

Additionally, the slicing workers lacked **panic recovery**, meaning any unexpected error (e.g., corrupted STL file causing an index out of bounds) would crash the entire application process.

## Fix Implemented

### 1. Concurrency Limit (Semaphore)
Added a semaphore to the `slicer.Engine` to limit the number of concurrent slicing jobs to **1**.
- If a new job is started while another is running, it enters a `pending` state with the message "Waiting in queue...".
- This prevents memory spikes from multiple simultaneous jobs.

### 2. Panic Recovery
Added `defer recover()` to the `sliceWorker` goroutine.
- If a panic occurs during slicing, it is caught.
- The job status is updated to `error` with the panic message.
- The server remains operational.

### 3. UI Protection
Modified `templates/slicer.templ` to use HTMX's `hx-disabled-elt`.
- The "Start Slice" button is automatically disabled as soon as it's clicked.
- It remains disabled until the server responds with the initial progress fragment.
- This prevents users from accidentally triggering multiple jobs.

### 4. Input Validation & Range Checks
Fixed a `runtime error: makeslice: len out of range` by adding strict validation for calculated values before they are used in `make()` calls:
- **Model Height**: Added a check to ensure the model is between 0.001mm and 10000mm.
- **Layer Count**: Capped the maximum number of layers to 1,000,000. This prevents integer overflow when casting huge floats to `int` (which previously resulted in negative numbers, causing the `makeslice` panic).
- **Anti-Aliasing**: Capped `aaLevel` to 8x.
- **Effective Resolution**: Added checks to ensure the product of resolution and AA level doesn't exceed reasonable limits (~1 billion pixels), preventing massive memory allocations that could trigger OOM or slice length errors.

## Verification
- Multiple rapid clicks on the "Start Slice" button now only trigger one request (UI-level protection).
- If multiple requests were somehow sent, they would be queued and executed sequentially (Engine-level protection).
- Memory usage is kept within safe limits by processing only one model at a time and validating image dimensions.
- Invalid model scales or extremely small layer heights now return a descriptive error message instead of crashing the server.

---

# Bugfix - Invalid STL Geometry (NaN/Inf Values) - Auto-Repair

## Problem Description

Users encountered an error during slicing:
```
Model is too large (+Inf mm). Maximum supported height is 10000 mm.
```
or
```
Invalid model geometry detected (mesh height is +Inf). The STL file may be corrupted or contain invalid vertex coordinates.
```

This error occurred when the mesh height calculation resulted in positive infinity (`+Inf`) instead of a valid numeric value.

## Root Cause Analysis

The issue was caused by **invalid vertex coordinates in STL files**. When an STL file contains:
- **Corrupted binary data** that interprets as IEEE 754 infinity or NaN values
- **Malformed ASCII STL** with invalid float representations
- **Export bugs** from 3D modeling software that produce non-finite coordinates

The STL parser would read these values without validation, causing:
1. Bounding box calculations (`MinBound`/`MaxBound`) to become `+Inf` or `-Inf`
2. Mesh height calculation (`MaxBound[2] - MinBound[2]`) to result in `+Inf`
3. The slicer to incorrectly report the model as "too large" instead of identifying the corrupted geometry

## Fix Implemented

### 1. STL Parser Auto-Repair (`internal/slicer/stl.go`)

Added `isValidFloat()` helper function that checks if a float32 value is valid:
```go
func isValidFloat(f float32) bool {
    return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}
```

**Binary STL parsing**: Each face is validated, and invalid faces are automatically removed:
```go
// Validate normal - skip face if normal is invalid
if !isValidFloat(tri.Normal[0]) || !isValidFloat(tri.Normal[1]) || !isValidFloat(tri.Normal[2]) {
    invalidFaces++
    continue
}

// Skip faces with invalid coordinates (NaN or Inf)
if !isValidFloat(x) || !isValidFloat(y) || !isValidFloat(z) {
    validFace = false
    break
}

if validFace {
    // Add triangle to mesh
    mesh.Triangles = append(mesh.Triangles, tri)
} else {
    invalidFaces++
}
```

**ASCII STL parsing**: Same repair logic applied during text parsing.

The parser now:
- **Automatically removes** triangles with invalid vertices (NaN or Inf)
- **Logs a warning** with the count of removed faces (visible in server console)
- **Continues processing** with the valid triangles
- Only fails if **no valid triangles** remain

### 2. Engine-Level Validation (`internal/slicer/engine.go`)

Added a secondary validation layer in the slicing engine to catch any edge cases:
```go
// Validate mesh height is not NaN or Infinite
if math.IsNaN(meshHeight) || math.IsInf(meshHeight, 0) {
    e.setError(job, fmt.Sprintf("Invalid model geometry detected (mesh height is %.2f). The STL file may be corrupted or contain invalid vertex coordinates.", meshHeight))
    return
}
```

This provides defense-in-depth: even if invalid data somehow reaches the engine, it will be caught before causing allocation errors or panics.

### 3. Graceful File Handling

When multiple files are selected for slicing:
- Invalid files are **skipped** with a warning logged
- Valid files continue to be processed
- User only sees an error if **all** files are invalid

## User Experience

**Auto-repair in action** (logged to server console, user sees normal progress):
```
Warning: Repaired STL by removing 5 invalid faces (kept 1243 triangles)
```

**If some files are invalid** (multi-file slicing):
- Valid files are sliced successfully
- Warning logged: `Warning: Only 2 of 3 files were valid and will be sliced`
- User sees successful completion

**If all files are invalid**:
```
No valid STL files could be parsed. Please check the files and try again.
```

**Engine-level validation** (if repair bypasses parser):
```
Invalid model geometry detected (mesh height is +Inf). The STL file may be corrupted or contain invalid vertex coordinates.
```

## Files Modified

| File | Changes |
|------|---------|
| `internal/slicer/stl.go` | Added `isValidFloat()` function, auto-repair in `parseSTLBinary()` and `parseSTLASCII()`, graceful handling of invalid faces |
| `internal/slicer/engine.go` | Added mesh height validation, improved file skip logic, only error if no valid files |

## Testing Recommendations

1. **Test with corrupted STL files** - Files with NaN/Inf vertices should now be repaired and sliced successfully
2. **Test with valid STL files** - Ensure normal slicing workflow is unaffected
3. **Test edge cases** - Models with very large but valid coordinates should still work (up to 10000mm limit)
4. **Test multi-file slicing** - Selecting a mix of valid and invalid files should process valid ones

## Future Improvements

Consider adding:
- **STL repair utilities** - Automatically fix common issues like inverted normals or small holes
- **Pre-flight validation** - Validate all STL files during the scan phase rather than waiting until slicing
- **Bounds checking during merge** - Validate bounds when merging multiple meshes to catch issues earlier
- **User notification** - Show a warning in the UI when files were auto-repaired

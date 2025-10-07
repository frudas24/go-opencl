# OpenCL bindings for Go

Documentation at <http://godoc.org/github.com/jgillich/go-opencl/cl>.

See the [test](cl/cl_test.go) for usage examples.

By default, the OpenCL 1.2 API is exported. To get OpenCL 1.0, set the build tag `cl10`.

---

## Updates (2025-10) contributed by frudas

The following improvements were added while keeping the original BSD 3-Clause license intact:

- `CommandQueue.Flush()` now calls `clFlush` (not `clFinish`), matching the OpenCL specification.
- Buffer/image helpers accept zero-length slices without panicking.
- `ImageDescription.Buffer` no longer triggers a panic when provided.
- `Device.OpenCLCMajorMinor()` exposes the parsed OpenCL C version reported by the driver.
- Build errors include per-device logs, and the program retains the device list after a successful build.
- Added a compatibility header `opencl_compat/CL/cl_kernel.h` for runtimes (e.g. NVIDIA NVVM) that expect legacy includes.

These changes aim to make the Go OpenCL binding safer and more convenient for any downstream project. Feel free to use or adapt them in your own forks; just keep the BSD license and attribution (see `LICENSE`).

//go:build darwin

package darwin

/*
#cgo pkg-config: soksak-process-identity
#include <stdlib.h>
#include <soksak_process_identity.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func (implementation) SetProcessDisplayName(displayName string) error {
	name := C.CString(displayName)
	defer C.free(unsafe.Pointer(name))
	var diagnostic [1024]C.char
	if status := C.soksak_process_set_display_name(name, &diagnostic[0], C.size_t(len(diagnostic))); status != 0 {
		return fmt.Errorf("LaunchServices registration failed with status %d: %s", int(status), C.GoString(&diagnostic[0]))
	}
	return nil
}

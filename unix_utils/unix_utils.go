// +build linux darwn freebsd openbsd 

package unix_utils

import (
	. "../flags"
	"strconv"

	"runtime"
	"syscall"
	"unsafe"

	// #include <stdlib.h>
	// #include <sys/types.h>
	// #include <pwd.h>
	"C"
)

// Calls the Stat() syscall for information
// about userid, groupid etc.. and tries to find the correct
// owner and groupname calling the getpwuid() syscall
// TODO: move to separate package to separate unsafe
//       code from safe code
func GetFileStat(fPath string, flags Flags) (stat syscall.Stat_t, ownername, groupname string, err error) {
	err = syscall.Stat(fPath, &stat)

	// if err == nil {
	// 	// stat syscall failed, return error
	// 	return stat, ownername, groupname, nil
	// }
	groupname = strconv.FormatUint(uint64(stat.Gid), int(flags.Base))
	ownername = strconv.FormatUint(uint64(stat.Uid), int(flags.Base))

	// fmt.Fprintf(os.Stderr, "Group: %d, Owner: %d\n", stat.Gid, stat.Uid)

	if runtime.GOOS == "linux" && !flags.Numeric_ids {
		// try getpwuid()
		passwd := C.getpwuid(C.__uid_t(stat.Uid))
		if uintptr(unsafe.Pointer(passwd)) != uintptr(0) {
			ownername = C.GoString(passwd.pw_name)
		}

		passwd = C.getpwuid(C.__uid_t(stat.Gid))
		if uintptr(unsafe.Pointer(passwd)) != uintptr(0) {
			groupname = C.GoString(passwd.pw_name)
		}
	}
	return stat, ownername, groupname, err
}

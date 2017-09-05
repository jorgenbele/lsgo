// A implementation of the 'ls' unix utility in go, using 'some' cgo

// TODO: -pretty print multi columns(done)-, support windows(?) + other os's,
// show symbolic links (required in various scripts, f.ex. the libre office
// launcher), remove Cgo dependencies

// NOTICE: There is currently a set of sorting methods available, but not all
// have commandline options. The default is set to AlphaTypeSort, which sorts by
// Type then by alphabetical ordering.

/// 02.06.2017 - Changed some things (for the worse) temporarily to make
/// this application useful for me.... printEntriesAdv is not done yet, but
/// it is useful for me. Must be changed in the future..
/// Did add some new sorting modes... Mainly SortByTypeAlpha, may be
/// removed in the future

// 27.08.2017 - Temporarily fixed printEntriesAdv, also added the files "." and "..".

// Author: Jorgen Bele <jorgen.bele@gmail.com>, 2016
// Date:   17.03.2016 (dd.mm.yyyy)

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	//	"math"

	"runtime"
	"syscall"
	"unicode/utf8"
	"unsafe"

	/*
		#if defined(__unix__) || defined(__APPLE__)

		#include <stdlib.h>
		#include <sys/types.h>
		#include <sys/stat.h>
		#include <pwd.h>
		#include <unistd.h>

		time_t access_time(const char *s) {
			struct stat st;
			stat(s, &st);
			return st.st_atime;
		}

		time_t mod_time(const char *s) {
			struct stat st;
			stat(s, &st);
			return st.st_mtime;
		}

		char *uid_to_s(unsigned int uid) {
			struct passwd *id = getpwuid(uid);
			if (!id) { return NULL; }
			return id->pw_name;
		}
		#endif
	*/
	"C"
)

// ioctl constants used by the ioctl syscall to get the terminal window size
const (
	TIOCGWINSZ     = 0x5413     // linux and others
	TIOCGWINSZ_OSX = 1074295912 // mac
)

type window struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func getTerminalWidth() (int, error) {
	w := new(window)
	tio := syscall.TIOCGWINSZ
	if runtime.GOOS == "darwin" {
		tio = TIOCGWINSZ_OSX
	}
	res, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(tio),
		uintptr(unsafe.Pointer(w)),
	)
	if int(res) == -1 {
		return 0, err
	}
	return int(w.Col), nil
}

func isTerminal() bool {
	return C.isatty(C.int(os.Stdout.Fd())) != 0
}

type Sort int

const (
	NoSort         Sort = iota // don't sort
	NumericSort                // numeric sort
	AlphaSort                  // alphabetical sort
	SizeSort                   // numeric sort on file size
	ModTimeSort                // last-modification sort
	AccessTimeSort             // last-access sort
	TypeSort                   // sort by file type
	TypeAlphaSort              // sort by file type then alphabetical order
)

type Flags struct {
	base      uint // numeric base (only for ints, not floating points)
	precision uint // numeric precision

	// printing settings
	sep       string // field seperator
	human     bool   // -h - human readable
	classify  bool   // -F - append character revealing filetype
	nolisting bool   // -d - shows info about file, but not its contents

	// sort
	sort    Sort // -f - sort type
	reverse bool // -r - list files in reverse

	// compatibility
	colors       bool // -G - use ansi colors
	interactive  bool // interactive or piped
	multi_column bool // -x - multi column output

	permissions  bool // print permissions
	size         bool // -s - file size
	recursive    bool // -R - recursively list for subdirectories
	all          bool // -a
	hide_obvious bool // -A, hide '.' and '..'

	group bool // print owner of file
	owner bool // print group of file

	inode       bool // -i - show inode number
	linkCount   bool // part of -l
	numeric_ids bool // -n - print owner/group as uid and gid instead of name

	non_graphic bool // -q - force print non graphic characters to screen
	reveal_dir  bool // -p - add / to end of directory names (as in -F)
}

var Args []string

func printError(err error) {
    s := err.Error()
    if s != "" {
        fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err.Error())
    }
}

// Returns the size of a file (binary size) in a
// format easily read by humans.
func sizeToHuman(size int64, base, precision uint) string {
	postfix := []string{"", "K", "M", "G", "T", "P", "E"}

	fsize := float64(size)
	var i int64 = 0

	for ; fsize > 1024; i++ {
		fsize /= 1024
	}

	if i != 0 && i < int64(len(postfix)) {
		return strconv.FormatFloat(fsize, 'f', int(precision), 64) + postfix[i]
	}

	return strconv.FormatInt(size, int(base))
}

// Calls the Stat() syscall for information
// about userid, groupid etc.. and tries to find the correct
// owner and groupname calling the getpwuid() syscall
// TODO: move to separate package, to separate unsafe
//       code from safe code
func getFileStat(fPath string, flags Flags) (stat syscall.Stat_t, ownername, groupname string, err error) {
	err = syscall.Stat(fPath, &stat) // ignoring error, but is returned to caller

	groupname = strconv.FormatUint(uint64(stat.Gid), int(flags.base))
	ownername = strconv.FormatUint(uint64(stat.Uid), int(flags.base))

	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd") && !flags.numeric_ids {
		c_group_name := C.uid_to_s(C.uint(stat.Gid))
		if unsafe.Pointer(c_group_name) != unsafe.Pointer(uintptr(0)) {
			groupname = C.GoString(c_group_name)
		}

		c_owner_name := C.uid_to_s(C.uint(stat.Uid))
		if unsafe.Pointer(c_owner_name) != unsafe.Pointer(uintptr(0)) {
			ownername = C.GoString(c_owner_name)
		}
	}
	return stat, ownername, groupname, err
}

const (
    RESET_ATTR = 0
    COLOR_FG_OFFSET = 30
    COLOR_BG_OFFSET = 40
)

const (
	COLOR_BLACK = iota + 1
	COLOR_RED
	COLOR_GREEN
	COLOR_YELLOW
	COLOR_BLUE
	COLOR_MAGENTA
	COLOR_CYAN
	COLOR_WHITE
)

func shellEscapeStr(s string, seq int) string {
    return fmt.Sprintf("\033[%0dm%s\033[0m", seq, s)
}


func colorFile(fPath string, mode os.FileMode) string {
	// socket
	if mode&os.ModeSocket != 0 {
		return shellEscapeStr(filepath.Clean(fPath), COLOR_CYAN+COLOR_FG_OFFSET)

		// fifo / named pipe
	} else if mode&os.ModeNamedPipe != 0 {
		return shellEscapeStr(filepath.Clean(fPath), COLOR_MAGENTA+COLOR_FG_OFFSET)

		// symbolic link
	} else if mode&os.ModeSymlink != 0 {
		return shellEscapeStr(filepath.Clean(fPath), COLOR_BLUE+COLOR_FG_OFFSET)

		// dir
	} else if mode&os.ModeDir != 0 {
		return shellEscapeStr(shellEscapeStr(filepath.Clean(fPath), COLOR_GREEN+COLOR_BG_OFFSET), COLOR_BLACK+COLOR_FG_OFFSET)

		// check if file is executable by someone
	} else if mode.Perm()&0111 != 0 {
		return shellEscapeStr(filepath.Clean(fPath), COLOR_GREEN+COLOR_FG_OFFSET)
	}

    return shellEscapeStr(filepath.Clean(fPath), RESET_ATTR)
	//return fPath
}

func classifyFile(fPath string, mode os.FileMode) string {

	// socket
	if mode&os.ModeSocket != 0 {
		return filepath.Clean(fPath) + "="

		// fifo / named pipe
	} else if mode&os.ModeNamedPipe != 0 {
		return filepath.Clean(fPath) + "|"

		// symbolic link
	} else if mode&os.ModeSymlink != 0 {
		return filepath.Clean(fPath) + "@"

		// dir
	} else if mode&os.ModeDir != 0 {
		return filepath.Clean(fPath) + "/"

		// check if file is executable by someone
	} else if mode.Perm()&0111 != 0 {
		return filepath.Clean(fPath) + "*"
	}

	return fPath
}


type LsFileInfo struct {
    name        string // filename
    path        string // full file path

    permissions string // permissions string (unix style)
    group       string // file group
    owner       string // file owner

    inode       string // inode count
    nLink       string // link count

    size        string // file size

    fileInfo    *os.FileInfo // pointer to FileInfo struct
}

func getLsFileInfo(fPath string, fileInfo *os.FileInfo, flags Flags) (LsFileInfo, error) {
    var lfi LsFileInfo

	// do not print if the -a flag is NOT specified and the
	// file starts with a . (dot) or ends with a ~ (tilda)
	if !flags.all {
		if strings.HasPrefix((*fileInfo).Name(), ".") || strings.HasSuffix((*fileInfo).Name(), "~") {
			return lfi, errors.New("")
		}
	}

	// do not print '.' and '..' the -A flag is specified (as opposed to -a),
	// as it sets the flags.hide_obvious to true)
	if flags.hide_obvious {
		if (*fileInfo).Name() == "." || (*fileInfo).Name() == ".." {
			return lfi, errors.New("")
		}
	}

	stat, ownername, groupname, _ := getFileStat(fPath, flags)
	if flags.inode {
		lfi.inode = strconv.FormatUint(uint64(stat.Ino), int(flags.base)) + flags.sep
	}

	if flags.permissions {
		perm := (*fileInfo).Mode().String()
		lfi.permissions = perm + flags.sep
	}

	// WARNING: unsafe
	if flags.linkCount {
		lfi.nLink = strconv.FormatUint(uint64(stat.Nlink), int(flags.base)) + flags.sep
	}

	if flags.owner {
		lfi.owner = ownername + flags.sep
	}

	if flags.group {
		lfi.group = groupname + flags.sep
	}

	if flags.size {
		size := (*fileInfo).Size()
		if flags.human {
			// the 'size' field with will always be at most 4 characters
			// wide + an extra character (K, M, G etc), if not considering
			// the precision (according to sizeToHuman)
			swidth := int(flags.precision + 4 + 1)

            lfi.size = fmt.Sprintf("%*s", swidth,
                sizeToHuman(size, flags.base, flags.precision))
		} else {
			lfi.size = fmt.Sprintf("%d", size)
		}
	}

    lfi.path = fPath

    // NOTICE: DOES NOT ADD COLORS!
    lfi.name = (*fileInfo).Name()

    lfi.fileInfo = fileInfo

    return lfi, nil
}

type LsFieldWidth struct {
    name        int // filename
    path        int // full file path

    permissions int // permissions string (unix style)
    group       int // file group
    owner       int // file owner

    inode       int // inode count
    nLink       int // link count

    size        int // file size

    fileInfo    *os.FileInfo // pointer to FileInfo struct
}

func getLsFieldWidth(infov []LsFileInfo) (LsFieldWidth) {
    var lfw LsFieldWidth

    max := func(x int, y int) int {
        if x > y {
            return x
        }else {
            return y
        }
    }

    for _, info := range infov {
        lfw.name = max(lfw.name, len(info.name))
        lfw.path = max(lfw.path, len(info.path))
        lfw.permissions = max(lfw.permissions, len(info.permissions))
        lfw.group = max(lfw.group, len(info.group))
        lfw.owner = max(lfw.owner, len(info.owner))
        lfw.inode = max(lfw.inode, len(info.inode))
        lfw.nLink = max(lfw.nLink, len(info.nLink))
        lfw.size = max(lfw.size, len(info.size))
    }
    return lfw
}

func compileLsString(lfi LsFileInfo, lfw LsFieldWidth, flags Flags) (string, error) {
    var fields []string

    if lfi.inode != "" {
        fields = append(fields, fmt.Sprintf("%*s", lfw.inode, lfi.inode))
    }

    if lfi.nLink != "" {
        fields = append(fields, fmt.Sprintf("%*s", lfw.nLink, lfi.nLink))
    }

    if lfi.permissions != "" {
        fields = append(fields, fmt.Sprintf("%-*s", lfw.permissions, lfi.permissions))
    }

    if lfi.owner != "" {
        fields = append(fields, fmt.Sprintf("%-*s", lfw.owner, lfi.owner))
    }

    if lfi.group != "" {
        fields = append(fields, fmt.Sprintf("%-*s", lfw.group, lfi.group))
    }

    if lfi.size != "" {
        fields = append(fields, fmt.Sprintf("%*s", lfw.size, lfi.size))
    }

    // Commented to disable adding name at this point.
//    if lfi.name != "" {
//        fields = append(fields, fmt.Sprintf("%-*s", lfw.name, lfi.name))
//    }

    s := strings.Join(fields, flags.sep)

    return s, nil
}


// Prints information about a file to standard output
// output according to flags
func getFileInfo(fPath string, file *os.FileInfo, flags Flags) (string, error) {
	info := ""

	// do not print if the -a flag is NOT specified and the
	// file starts with a . (dot) or ends with a ~ (tilda)
	if !flags.all {
		if strings.HasPrefix((*file).Name(), ".") || strings.HasSuffix((*file).Name(), "~") {
			return info, errors.New("")
		}
	}

	// do not print '.' and '..' the -A flag is specified (as opposed to -a),
	// as it sets the flags.hide_obvious to true)
	if flags.hide_obvious {
		if (*file).Name() == "." || (*file).Name() == ".." {
			return info, errors.New("")
		}
	}

	// retrive additional information about the file, *UNSAFE*
	stat, ownername, groupname, _ := getFileStat(fPath, flags)

	if flags.inode {
		info += strconv.FormatUint(uint64(stat.Ino), int(flags.base)) + flags.sep
	}

	if flags.permissions {
		perm := (*file).Mode().String()
		info += perm + flags.sep
	}

	// WARNING: unsafe
	if flags.linkCount {
		info += strconv.FormatUint(uint64(stat.Nlink), int(flags.base)) + flags.sep
	}

	if flags.owner {
		info += ownername + flags.sep
	}

	if flags.group {
		info += groupname + flags.sep
	}

	if flags.size {
		size := (*file).Size()
		if flags.human {
			// the 'size' field with will always be at most 4 characters
			// wide + an extra character (K, M, G etc), if not considering
			// the precision (according to sizeToHuman)
			swidth := int(flags.precision + 4 + 1)

			// if printing multiple entries per line, do not include uneccesary whitespace
			if flags.multi_column {
				info += fmt.Sprintf("%*s", swidth,
					sizeToHuman(size, flags.base, flags.precision))
			} else {
				info += fmt.Sprintf("%*s", swidth,
					sizeToHuman(size, flags.base, flags.precision))
			}

		} else {
			info += fmt.Sprintf("%d", size)
		}
	}

	if info != "" {
		info += flags.sep
	}

	_, relativePath := filepath.Split(fPath)

	if flags.classify {
		relativePath = classifyFile(relativePath, (*file).Mode())
	} else if flags.reveal_dir && (*file).IsDir() {
		relativePath = filepath.Clean(relativePath) + "/"
	}

    if flags.colors {
        relativePath = colorFile(relativePath, (*file).Mode())
    }

	return info + relativePath, nil
}

// sort interfaces
type sortByAlpha []os.FileInfo

func (a sortByAlpha) Len() int           { return len(a) }
func (a sortByAlpha) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByAlpha) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

type sortByModTime []os.FileInfo

func (a sortByModTime) Len() int           { return len(a) }
func (a sortByModTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByModTime) Less(i, j int) bool { return a[i].ModTime().Before(a[j].ModTime()) }

type sortByAccessTime []os.FileInfo

func (a sortByAccessTime) Len() int      { return len(a) }
func (a sortByAccessTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByAccessTime) Less(i, j int) bool {
	accessTime := func(fPath string) C.time_t {
		return C.access_time(C.CString(fPath))
	}
	return accessTime(a[i].Name()) < accessTime(a[i].Name())
}

type sortBySize []os.FileInfo

func (a sortBySize) Len() int           { return len(a) }
func (a sortBySize) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortBySize) Less(i, j int) bool { return a[i].Size() < a[j].Size() }

type sortByType []os.FileInfo

func (a sortByType) Len() int      { return len(a) }
func (a sortByType) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByType) Less(i, j int) bool {
	return filetypeSortValue(a[i].Mode()) < filetypeSortValue(a[j].Mode())
}

type sortByTypeAlpha []os.FileInfo

func (a sortByTypeAlpha) Len() int      { return len(a) }
func (a sortByTypeAlpha) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByTypeAlpha) Less(i, j int) bool {
	// Sort first based on type, then alphabetical order
	vi := filetypeSortValue(a[i].Mode())
	vj := filetypeSortValue(a[j].Mode())

	if vi != vj {
		return vi > vj
	}

	return a[i].Name() < a[j].Name()
}

// TODO: Clean up this mess.
func filetypeSortValue(mode os.FileMode) uint {

	// dir
	if mode.IsDir() {
		return 6

		// check if file is executable by someone
	} else if mode.Perm()&0111 != 0 {
		return 5

		// socket
	} else if mode&os.ModeSocket != 0 {
		return 4

		// fifo / named pipe
	} else if mode&os.ModeNamedPipe != 0 {
		return 3

		// symbolic link
	} else if mode&os.ModeSymlink != 0 {
		return 2

	} else if mode.IsRegular() {
		return 1
	}

	return 10
}

// Sorts the []os.FileInfo slice by flags
func sortDirs(dir []os.FileInfo, flags Flags) []os.FileInfo {

	switch flags.sort {
	case AlphaSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortByAlpha(dir)))
		} else {
			sort.Sort(sortByAlpha(dir))
		}

	case ModTimeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortByModTime(dir)))
		} else {
			sort.Sort(sortByModTime(dir))
		}
	case AccessTimeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortByAccessTime(dir)))
		} else {
			sort.Sort(sortByAccessTime(dir))
		}

	case SizeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortBySize(dir)))
		} else {
			sort.Sort(sortBySize(dir))
		}

	case TypeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortByType(dir)))
		} else {
			sort.Sort(sortByType(dir))
		}

	case TypeAlphaSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(sortByTypeAlpha(dir)))
		} else {
			sort.Sort(sortByTypeAlpha(dir))
		}
	}

	return dir
}

const StringSeparators = " "

func printDirEntriesAdv(fPath string, dir []os.FileInfo, flags Flags) bool {
	// sort directory listing
	dir = sortDirs(dir, flags)

	cols, err := getTerminalWidth()

	if err != nil {
		cols = 80 // default
	}

	getEntry := func(path string, relative bool) (LsFileInfo, error) {
		var cPath string

		if filepath.IsAbs(path) || relative {
			cPath = filepath.Clean(path)
		} else {
			cPath = filepath.Join(fPath, path)
		}

		info, err := os.Stat(cPath)
		if err != nil {
            var lfi LsFileInfo
			return lfi, err
		}

		return getLsFileInfo(cPath, &info, flags)
    }

	var entries []LsFileInfo
	var dirs []LsFileInfo // list of sub-directories

	// append '.' and '..' directories 
    if flags.all && !flags.hide_obvious {
        oldCDW, _ := os.Getwd()
        os.Chdir(fPath)
        entryCurrent, err := getEntry(".", true)
        if err == nil {
            entries = append(entries, entryCurrent)
        }
        entryParent, err := getEntry("..", true)
        if err == nil && entryCurrent != entryParent {
            entries = append(entries, entryParent)
        }
        os.Chdir(oldCDW)
    }

	for _, fileInfo := range dir {
		entry, err := getEntry(fileInfo.Name(), false)
		if err != nil {
            printError(err)
            continue
        }

        entries = append(entries, entry)

        // append to list of directories
        if fileInfo.IsDir() {
            dirs = append(dirs, entry)
        }
	}

//    lfw := getLsFieldWidth(entries)

//    var lsEntries []string
//    for _, info := range entries {
//        s, err := compileLsString(info, lfw, flags)
//        if err != nil {
//            continue
//        }
//        lsEntries = append(lsEntries, s)
//    }

//	lMax, _, lSum := maxEntryWidth(lsEntries)

    //topCDW, _ := os.Getwd()
    //os.Chdir(fPath)
//    prettyPrintEntries(lsEntries, flags, cols, lMax, lSum)
    //os.Chdir(topCDW)

    prettyPrintEntriesAdv(entries, cols, flags)

    // recursively print for sub-directories
    if flags.recursive {
        topCDW, _ := os.Getwd()
        os.Chdir(fPath)
        for _, d := range dirs {
            df, err := os.Open(d.path)
            defer df.Close()
            if err != nil {
                printError(err)
                continue
            }

            dlist, err := df.Readdir(0)
            if err != nil {
                printError(err)
                continue
            }

            if flags.colors {
                fmt.Printf("\n%s:\n", colorFile(d.name, (*d.fileInfo).Mode()))
            } else {
                fmt.Printf("\n%s:\n", d.name)
            }

            printDirEntriesAdv(d.path, dlist, flags)
        }
        os.Chdir(topCDW)
    }

	return true // TODO: fix return value
}

func maxEntryWidth(entries []string) (lMax, lMin, lSum int) {
	// avg := sum / sum_n
	lMax = int(0)
	lMin = int(^uint(0) >> 1)
	lSum = int(0)

	for _, s := range entries {
		len := utf8.RuneCountInString(s)

		lSum += len

		if len < lMin {
			lMin = len
		}
        if len > lMax {
			lMax = len
		}
	}
	return
}

func prettyPrintEntriesAdv(entries []LsFileInfo, cols int, flags Flags) bool {
    lfw := getLsFieldWidth(entries)

	if !flags.multi_column {
		for _, entry := range entries {
            lsInfo, _ := compileLsString(entry, lfw, flags)
            var name string
            if flags.colors {
                name = colorFile(entry.name, (*entry.fileInfo).Mode())
                if flags.classify {
                    name = classifyFile(name, (*entry.fileInfo).Mode())
                }
            } else {
                name = entry.name
            }

            if len(lsInfo) > 0 {
                fmt.Printf("%s%s", lsInfo, flags.sep)
            }
            fmt.Printf("%s\n", name)
		}
	} else {
        col := 0
        row := 0

        for _, entry := range entries {
            lsInfo, _ := compileLsString(entry, lfw, flags)

            var name string
            if flags.colors {
                name = colorFile(entry.name, (*entry.fileInfo).Mode())
                if flags.classify {
                    name = classifyFile(name, (*entry.fileInfo).Mode())
                }
            } else {
                name = entry.name
            }

            length := len(lsInfo) + lfw.name
            if len(lsInfo) > 0 {
                length += len(flags.sep)
            }
            if len(name) > lfw.name {
                length += len(name) - lfw.name
            }

            if col + length >= cols {
                col = 0
                row++
                fmt.Println()
            }

            if col > 0 {
                fmt.Print(flags.sep)
            }

            if len(lsInfo) > 0 {
                fmt.Printf("%s%s", lsInfo, flags.sep)
            }
            fmt.Printf("%s", name)

            for i := 0; i < lfw.name - len(entry.name); i++ {
                fmt.Print(" ")
            }

            col += length
        }

        if col > 0 {
            fmt.Println()
        }
    }
	return true
}


func prettyPrintEntries(entries []string, flags Flags, cols int, lMax int, lSum int) bool {
	// if lMax < cols || flags.multi_column {
	if !flags.multi_column {
		for _, s := range entries {
            fmt.Printf("%s\n", s)
		}
    } else if lSum < cols {
        i := 0
		for _, s := range entries {
            if i > 0 {
                fmt.Printf(flags.sep)
            }
            fmt.Printf("%*s", lMax, s)
            i++
		}
        if i > 0 {
            fmt.Println()
        }
	} else {
		return printByColumnWidth(entries, lMax+1, cols, flags)
	}
	return true
}

// printByColumnWidth: prints the contents in several columns, sorted horizontally
// (as opposed to GNU's version of ´ls´ which sorts vertically)
// (this is both because it is easier, and because I personally prefer it)
func printByColumnWidth(entries []string, colWidth int, width int, flags Flags) bool {
    fmt.Printf("|")

	col := 0
	row := 0

	for _, s := range entries {

		// len := utf8.RuneCountInString(s)

		if col+colWidth < width {
			if col > 0 {
				fmt.Printf(flags.sep)
				col++
			}
		} else {
			fmt.Println()
			col = 0
			row++
		}

		fmt.Printf("|%*s|", colWidth, s)
		//fmt.Printf("|%s|", s)
		col += colWidth + len(flags.sep)
	}

	if col > 0 {
		fmt.Println()
	}

	return true
}

// Prints the directory list to standard
// output according to flags
// TODO: fix the display of . and .. when -a is enabled
func printDirEntries(fPath string, dir []os.FileInfo, flags Flags) bool {
	// sort directory listing
	dir = sortDirs(dir, flags)
	var paths []string

	cols, err := getTerminalWidth()

	if err != nil {
		cols = 80 // default
	}

	// current column, used in deciding if an entry can fit on the line
	col := 0
	// number of lines printed, used in deciding if needed to print a newline or not
	printed := 0

	printNext := func(s string) {
		col += len(s) + len(flags.sep)

		if col <= cols && flags.multi_column {
			fmt.Printf("%s%s", s, flags.sep)
		} else {
			if printed > 0 {
				fmt.Println()
			}
			col = len(s) + len(flags.sep)
			fmt.Printf("%s%s", s, flags.sep)
		}
		printed++
	}

	// print files in directory

    // store file info in a list, this is used to find
    // the largest field lengths for better formatting
    var infov []LsFileInfo

	for _, file := range dir {
		var cPath string

		if filepath.IsAbs(file.Name()) {
			cPath = filepath.Clean(file.Name())
		} else {
			cPath = filepath.Join(fPath, file.Name())
		}

		info, err := getLsFileInfo(cPath, &file, flags)

		if err != nil {
			continue
		}

        infov = append(infov, info)
        //lsStr, err := compilLsString(info)

		//printNext(info)
        //paths = append(paths, cPath)
	}

    lfw := getLsFieldWidth(infov)

    for _, info := range infov {
        s, err := compileLsString(info, lfw, flags)
        if err != nil {
            continue
        }
        printNext(s)
    }

	// recurse into all sub-directories if recursive mode is enabled
	if flags.recursive {
		for _, v := range paths {
			// check if the path is to a directory
			finfo, err := os.Lstat(v)

			if err != nil {
				printError(err)
				continue
			} else if !finfo.IsDir() {
				continue
			} else if finfo.Mode() == os.ModeSymlink && flags.nolisting {
				info, err := getFileInfo(v, &finfo, flags)
				if err != nil {
					continue
				}
				printNext(info)
				continue
			}

			// open directory
			dir, err := os.Open(v)
			defer dir.Close()

			if err != nil {
				printError(err)
				continue
			}

			// read directory contents list
			dlist, err := dir.Readdir(0)

			if err != nil {
				printError(err)
				dir.Close()
				continue
			}

			// all ok, close dir file, then recursively print its entries
			fmt.Printf("\n%s:\n", v)
			printDirEntries(v, dlist, flags)
		}
	}

	if col > 0 {
		fmt.Println()
	}
	return true
}

// Parses command-line arguments in posix-style
// [-flags] [files...]
func parseArgs() Flags {
	// initialize default config
	var flags Flags
	flags.base = 10
	flags.precision = 2
	flags.sort = TypeAlphaSort // see notice at the top of the file
	flags.interactive = isTerminal()
	flags.multi_column = flags.interactive
	//flags.colors = flags.interactive
	flags.sep = "\t" // tab
    flags.recursive = false
	//flags.sep = " " // space

	reoa := false // reached end of arguments

	var i int
	for i = 1; i < len(os.Args); i++ {

		if os.Args[i][0] == '-' && !reoa {
			for _, c := range os.Args[i][1:] {
				if reoa {
					break
				}

				switch c {
				case '-':
					reoa = true

				case 'l': // long format, displaying Unix file types etc...
					flags.permissions = true
					flags.linkCount = true
					flags.size = true
					flags.group = true
					flags.owner = true
					flags.multi_column = false

				case 'o':
					flags.permissions = true
					flags.linkCount = true
					flags.size = true
					flags.group = true
					flags.owner = false
					flags.multi_column = false

					// ...
				case 'F': // reveal nature of a file
					flags.classify = true

				case 'f': // do not sort
					flags.sort = NoSort
					//fallthrough

				case 'A': // list all files except . and ..
					flags.all = true
					flags.hide_obvious = true

				case 'a': // list all files
					flags.all = true
					flags.hide_obvious = false

				case 'r':
					flags.reverse = true
				case 'R': // recursively list
					flags.recursive = true

				case 'd': // info about file, not contents
					flags.nolisting = true

				case 't': // sort by mod time
					flags.sort = ModTimeSort

				case 'g':
					flags.permissions = true
					flags.size = true
					flags.group = true
					flags.multi_column = false

				case 'q':
					flags.non_graphic = true

				case 'p':
					flags.reveal_dir = true

				case 'u':
					flags.sort = AccessTimeSort

				case 'm':
					flags.sep = ","
					flags.multi_column = true

				case 's': // size
					flags.size = true

				case 'S': // sort by size
					flags.sort = SizeSort

				case 'n': // numeric ids
					flags.numeric_ids = true
					flags.permissions = true
					flags.size = true
					flags.group = true
					flags.owner = true
					flags.multi_column = false

				case 'i': // inode number
					flags.inode = true

				case 'h': // human
					flags.human = true

				case 'x': // multi-column output
					flags.multi_column = true
				case '1': // one column per line
					flags.multi_column = false

				case 'G': // colors
					flags.colors = true

				default:
					err := errors.New("unknown flag " + os.Args[0] + " ignoring.")
					printError(err)
				}
			}
		} else {
			// stop at first non-flag argument, and threat the rest as files
			break
		}
	}

	// treat the rest of the arguments as files
	for ; i < len(os.Args); i++ {
		Args = append(Args, os.Args[i])
	}

	if flags.human && !flags.size {
		fmt.Printf("Usage: %s [-1acdfFghilmnpqRsStux] [file ...]\n", os.Args[0])
		os.Exit(1)
	}

	if flags.multi_column && flags.sep != "," {
		flags.sep = " "
	}

	return flags
}

func main() {
	flags := parseArgs()

	if len(Args) == 0 {
		Args = append(Args, ".") // current directory
	}

	var dirs []string
	var files []string

	for _, arg := range Args {
		// clean path
		arg = filepath.Clean(arg)

		finfo, err := os.Stat(arg)

		if err != nil {
			printError(err)
			continue
		}

		if finfo.IsDir() {
			dirs = append(dirs, arg)
		} else {
			files = append(files, arg)
		}
	}

	if flags.multi_column {
		cols, err := getTerminalWidth()
		if err != nil {
			cols = 80 // default
		}
		lMax, _, lSum := maxEntryWidth(files)
		prettyPrintEntries(files, flags, cols, lMax, lSum)
	} else {
		for _, filePath := range files {
			finfo, err := os.Stat(filePath)
			if err != nil {
				printError(err)
				continue
			}
			info, _ := getFileInfo(filePath, &finfo, flags)
			fmt.Println(info)
		}
	}

	// separate last directory entry from list of single files
	if len(files) > 0 {
		fmt.Println()
		if flags.multi_column {
			fmt.Println()
		}
	}

	for i, filePath := range dirs {
		file, err := os.Open(filePath)
		defer file.Close()

		if err != nil {
			printError(err)
			continue
		}

		if len(dirs) > 1 {
			fmt.Printf("%s:\n", file.Name())
		}
		dir, err := file.Readdir(0)

		if err != nil {
            printError(err)
			continue
		}

		printDirEntriesAdv(filePath, dir, flags)

		// print newline to seperate the next directory printed
		if i+1 < len(dirs) {
			fmt.Println()
		}
	}
}

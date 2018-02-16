// TODO: -pretty print multi columns(done)-, support windows(?) + other os's,
// show symbolic links (required in various scripts, f.ex. the libre office
// launcher), remove Cgo dependencies

// NOTICE: There is currently a set of sorting methods available, but not all
// have commandline options. The default is set to AlphatypeSort, which sorts by
// Type then by alphabetical ordering.

/// 02.06.2017 - Changed some things (for the worse) temporarily to make
/// this application useful for me.... printEntriesAdv is not done yet, but
/// it is useful for me. Must be changed in the future..
/// Did add some new sorting modes... Mainly SortByTypeAlpha, may be
/// removed in the future

// 27.08.2017 - Temporarily fixed printEntriesAdv, also added the files "." and "..".

// 06.09.2017 - Finally fixed color support, cleaned up the code - removing unecessary functions, changed os.Stat() with os.Lstat().

// Author: Jorgen Bele <jorgen.bele@gmail.com>, 2016
// Date:   17.03.2016 (dd.mm.yyyy)

// http://pubs.opengroup.org/onlinepubs/009695399/utilities/ls.html

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"time"

	"runtime"
	"syscall"
	//"unicode/utf8"
	"unsafe"

	/*
		#if defined(__unix__) || defined(__APPLE__)

		#include <stdlib.h>
		#include <sys/types.h>
		#include <sys/stat.h>
		#include <pwd.h>
		#include <grp.h>
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

		char *gid_to_s(unsigned int gid) {
			struct group *id = getgrgid(gid);
			if (!id) { return NULL; }
			return id->gr_name;
		}
		#endif
	*/
	"C"
)

// ioctl constants used by the ioctl syscall to get the terminal window size
const (
	//TIOCGWINSZ     = 0x5413     // linux and others
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

type sortType int

// Sorting types
const (
	noSort         sortType = iota // don't sort
	numericSort                    // numeric sort
	alphaSort                      // alphabetical sort
	sizeSort                       // numeric sort on file size
	modTimeSort                    // last-modification sort
	accessTimeSort                 // last-access sort
	typeSort                       // sort by file type
	typeAlphaSort                  // sort by file type then alphabetical order
)

type flags struct {
	base      uint // numeric base (only for ints, not floating points)
	precision uint // numeric precision

	// printing settings
	sep       string // field seperator
	human     bool   // -h - human readable
	classify  bool   // -F - append character revealing filetype
	nolisting bool   // -d - shows info about file, but not its contents

	// switches
	recursive       bool // -R - recursively list for subdirectories
	all             bool // -a
	hideObvious     bool // -A, hide '.' and '..'
	date            bool // show date
	dereferenceLink bool // -L - display information about the target of the link instead of the link iteslf

	// sort
	sort    sortType // -f - sort type
	reverse bool     // -r - list files in reverse

	// compatibility
	colors      bool // -G - use ansi colors
	interactive bool // interactive or piped
	multiColumn bool // -x - multi column output

	size bool // -s - file size

	// ownership
	group       bool // print owner of file
	owner       bool // print group of file
	permissions bool // print permissions

	// internal
	inode      bool // -i - show inode number
	linkCount  bool // part of -l - link count
	numericIds bool // -n - print owner/group as uid and gid instead of name

	nonGraphic bool // -q - force print non graphic characters to screen
	revealDir  bool // -p - add / to end of directory names (as in -F)
}

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
	var i int

	for ; fsize > 1024; i++ {
		fsize /= 1024
	}

	if i != 0 && i < len(postfix) {
		return strconv.FormatFloat(fsize, 'f', int(precision), 64) + postfix[i]
	}

	return strconv.FormatInt(size, int(base))
}

// Calls the Stat() syscall for information
// about userid, groupid etc.. and tries to find the correct
// owner and groupname calling the getpwuid() syscall
// TODO: move to separate package, to separate unsafe
//       code from safe code
func getFileStat(fPath string, flags flags) (stat syscall.Stat_t, ownername, groupname string, err error) {
	err = syscall.Stat(fPath, &stat) // ignoring error, but is returned to caller

	if (runtime.GOOS == "unix" || runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd") && !flags.numericIds {
		cGroupName := C.gid_to_s(C.uint(stat.Gid))
		if unsafe.Pointer(cGroupName) != unsafe.Pointer(uintptr(0)) {
			groupname = C.GoString(cGroupName)
		}

		cOwnerName := C.uid_to_s(C.uint(stat.Uid))
		if unsafe.Pointer(cOwnerName) != unsafe.Pointer(uintptr(0)) {
			ownername = C.GoString(cOwnerName)
		}
		return stat, ownername, groupname, err
	} else {
		// use uids
		groupname = strconv.FormatUint(uint64(stat.Gid), int(flags.base))
		ownername = strconv.FormatUint(uint64(stat.Uid), int(flags.base))
	}
	return stat, ownername, groupname, err
}

const (
	resetAttr          = 0
	colorFgOffset      = 30
	colorFgLightOffset = 90
	colorBgOffset      = 40
	colorBgLightOffset = 100
)

const (
	colorBlack = iota
	colorRed
	colorGreen
	colorYellow
	colorBlue
	colorMagenta
	colorCyan
	colorLightGrey
)

func shellEscapeStr(s string, seq int) string {
	return fmt.Sprintf("\033[%0dm%s\033[0m", seq, s)
}

func colorFile(fPath string, mode os.FileMode) string {
	// socket
	if mode&os.ModeSocket != 0 {
		fPath = shellEscapeStr(filepath.Clean(fPath), colorCyan+colorFgOffset)

		// fifo / named pipe
	} else if mode&os.ModeNamedPipe != 0 {
		fPath = shellEscapeStr(filepath.Clean(fPath), colorMagenta+colorFgOffset)

		// symbolic link
	} else if mode&os.ModeSymlink != 0 {
		fPath = shellEscapeStr(filepath.Clean(fPath), colorBlue+colorFgOffset)

		// dir
	} else if mode&os.ModeDir != 0 {
		if mode.Perm()&0111 == 0111 { // read, write and executable
			fPath = shellEscapeStr(shellEscapeStr(filepath.Clean(fPath), colorGreen+colorBgOffset), colorBlack+colorFgOffset)
		} else {
			fPath = shellEscapeStr(filepath.Clean(fPath), colorBlue+colorFgLightOffset)
		}
		// check if file is executable by someone
	} else if mode.Perm()&0111 != 0 {
		fPath = shellEscapeStr(filepath.Clean(fPath), colorGreen+colorFgOffset)
	} else {
		fPath = shellEscapeStr(filepath.Clean(fPath), resetAttr)
	}

	return fPath
	//return fPath
}

func classifyFile(fPath string, mode os.FileMode) string {
	postfix := " "

	// socket
	if mode&os.ModeSymlink != 0 {
		postfix = "@"
	} else if mode&os.ModeSocket != 0 {
		postfix = "="
		// fifo / named pipe
	} else if mode&os.ModeNamedPipe != 0 {
		postfix = "|"
		// dir
	} else if mode&os.ModeDir != 0 {
		postfix = "/"
		// check if file is executable by someone
	} else if mode.Perm()&0111 != 0 {
		postfix = "*"
	}

	return filepath.Clean(fPath) + postfix
}

type lsFileInfo struct {
	name string // filename
	path string // full file path

	permissions string // permissions string (unix style)
	group       string // file group
	owner       string // file owner

	inode string // inode count
	nLink string // link count

	size string // file size

	modMonth      string // month
	modDate       string // date
	modYear       string // year
	modHourMinute string // second

	fileInfo *os.FileInfo // pointer to FileInfo struct
}

func getlsFileInfo(fPath string, fileInfo *os.FileInfo, flags flags) (lsFileInfo, error) {
	var lfi lsFileInfo

	// do not print if the -a flag is NOT specified and the
	// file starts with a . (dot) or ends with a ~ (tilda)
	if !flags.all {
		if strings.HasPrefix((*fileInfo).Name(), ".") || strings.HasSuffix((*fileInfo).Name(), "~") {
			return lfi, errors.New("")
		}
	}

	// do not print '.' and '..' the -A flag is specified (as opposed to -a),
	// as it sets the flags.hideObvious to true)
	if flags.hideObvious {
		if (*fileInfo).Name() == "." || (*fileInfo).Name() == ".." {
			return lfi, errors.New("")
		}
	}

	stat, ownername, groupname, _ := getFileStat(fPath, flags)
	if flags.inode {
		lfi.inode = strconv.FormatUint(stat.Ino, int(flags.base))
	}

	if flags.permissions {
		perm := (*fileInfo).Mode().String()
		lfi.permissions = perm
	}

	// WARNING: unsafe
	if flags.linkCount {
		// stat.Nlink is implementation dependent, convert to uint64
		lfi.nLink = strconv.FormatUint(uint64(stat.Nlink), int(flags.base))
	}

	if flags.owner {
		lfi.owner = ownername
	}

	if flags.group {
		lfi.group = groupname
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

	if flags.date {
		modTime := (*fileInfo).ModTime()
		now := time.Now()

		secondsPerSixMonths := int64(365 * 60 * 60 * 24 / 2)
		// if file was modified within the last six months, print in the format: "<Month> <date> <hour:minute>"
		if time.Unix(now.Unix()-secondsPerSixMonths, 0).Before(modTime) {
			lfi.modHourMinute = modTime.Format("15:04")
		} else {
			lfi.modYear = modTime.Format(" 2006")
		}

		lfi.modMonth = modTime.Format("Jan")
		lfi.modDate = modTime.Format(" 2")
	}

	lfi.path = fPath

	// NOTICE: DOES NOT ADD COLORS!
	lfi.name = (*fileInfo).Name()

	lfi.fileInfo = fileInfo

	return lfi, nil
}

type lsFieldWidth struct {
	name int // filename
	path int // full file path

	permissions int // permissions string (unix style)
	group       int // file group
	owner       int // file owner

	inode int // inode count
	nLink int // link count

	size int // file size

	modMonth      int // month
	modDate       int // date
	modYear       int // year
	modHourMinute int // hour:minute
}

func getlsFieldWidth(infov []lsFileInfo) lsFieldWidth {
	max := func(x int, y int) int {
		if x > y {
			return x
		}
		return y
	}

	var lfw lsFieldWidth
	for _, info := range infov {
		lfw.name = max(lfw.name, len(info.name))
		lfw.path = max(lfw.path, len(info.path))
		lfw.permissions = max(lfw.permissions, len(info.permissions))
		lfw.group = max(lfw.group, len(info.group))
		lfw.owner = max(lfw.owner, len(info.owner))
		lfw.inode = max(lfw.inode, len(info.inode))
		lfw.nLink = max(lfw.nLink, len(info.nLink))
		lfw.size = max(lfw.size, len(info.size))
		lfw.modMonth = max(lfw.modMonth, len(info.modMonth))
		lfw.modDate = max(lfw.modDate, len(info.modDate))
		lfw.modYear = max(lfw.modYear, len(info.modYear))
		lfw.modHourMinute = max(lfw.modHourMinute, len(info.modHourMinute))
	}
	return lfw
}

func compileLsStringSimple(lfi lsFileInfo, flags flags) (string, error) {
	var fields []string

	if lfi.inode != "" {
		fields = append(fields, lfi.inode)
	}

	if lfi.permissions != "" {
		fields = append(fields, lfi.permissions)
	}

	if lfi.nLink != "" {
		fields = append(fields, lfi.nLink)
	}

	if lfi.owner != "" {
		fields = append(fields, lfi.owner)
	}

	if lfi.group != "" {
		fields = append(fields, lfi.group)
	}

	if lfi.size != "" {
		fields = append(fields, lfi.size)
	}

	// Modification time
	if lfi.modMonth != "" {
		fields = append(fields, lfi.modMonth)
	}
	if lfi.modDate != "" {
		fields = append(fields, lfi.modDate)
	}
	if lfi.modYear != "" {
		fields = append(fields, lfi.modYear)
	}
	if lfi.modHourMinute != "" {
		fields = append(fields, lfi.modHourMinute)
	}
	s := strings.Join(fields, flags.sep)

	return s, nil
}

// WARNING: does not add name to string!
func compileLsString(lfi lsFileInfo, lfw lsFieldWidth, flags flags) (string, error) {
	var fields []string

	if lfi.inode != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.inode, lfi.inode))
	}

	if lfi.permissions != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.permissions, lfi.permissions))
	}

	if lfi.nLink != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.nLink, lfi.nLink))
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

	// Modification time
	if lfi.modMonth != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.modMonth, lfi.modMonth))
	}
	if lfi.modDate != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.modDate, lfi.modDate))
	}
	if lfi.modYear != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.modYear, lfi.modYear))
	}
	if lfi.modHourMinute != "" {
		fields = append(fields, fmt.Sprintf("%*s", lfw.modHourMinute, lfi.modHourMinute))
	}
	s := strings.Join(fields, flags.sep)

	return s, nil
}

// sort interfaces
type sortByAlpha []os.FileInfo

func (a sortByAlpha) Len() int           { return len(a) }
func (a sortByAlpha) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortByAlpha) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

type sortByNumeric []os.FileInfo

func (a sortByNumeric) Len() int      { return len(a) }
func (a sortByNumeric) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sortByNumeric) Less(i, j int) bool {
	v1, _ := strconv.Atoi(a[i].Name())
	v2, _ := strconv.Atoi(a[j].Name())
	return v1 < v2
}

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
	return accessTime(a[i].Name()) < accessTime(a[j].Name())
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
func sortDirs(dir []os.FileInfo, flags flags) []os.FileInfo {
	switch flags.sort {
	case alphaSort:
		sort.Sort(sortByAlpha(dir))
	case modTimeSort:
		sort.Sort(sortByModTime(dir))
	case accessTimeSort:
		sort.Sort(sortByAccessTime(dir))
	case sizeSort:
		sort.Sort(sortBySize(dir))
	case typeSort:
		sort.Sort(sortByType(dir))
	case typeAlphaSort:
		sort.Sort(sortByTypeAlpha(dir))
	}

	reverse := func(dir *[]os.FileInfo) *[]os.FileInfo {
		for i, j := 0, len(*dir)-1; i < j; i, j = i+1, j-1 {
			(*dir)[i], (*dir)[j] = (*dir)[j], (*dir)[i]
		}
		return dir
	}
	if flags.reverse {
		reverse(&dir)
	}

	return dir
}

func getlsFileInfoEntry(root string, path string, relative bool, flags flags) (lsFileInfo, error) {
	var cPath string

	if !filepath.IsAbs(path) || relative {
		cPath = filepath.Join(root, path)
	} else {
		cPath = filepath.Clean(path)
	}

	var info os.FileInfo
	var err error
	if flags.dereferenceLink {
		info, err = os.Stat(cPath)
	} else {
		info, err = os.Lstat(cPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cPath: %s, getlsFileInfoEntry: err != nil: %v\n", cPath, err)
		var lfi lsFileInfo
		return lfi, err
	}
	return getlsFileInfo(cPath, &info, flags)
}

func printDirEntriesAdv(fPath string, dir []os.FileInfo, flags flags) bool {
	cols, err := getTerminalWidth()

	if err != nil {
		cols = 80 // default
	}

	var entries []lsFileInfo
	var dirs []lsFileInfo // list of sub-directories

	// append '.' and '..' directories
	if flags.all && !flags.hideObvious {
		entryCurrent, err := getlsFileInfoEntry("", ".", true, flags)
		if err == nil {
			entries = append(entries, entryCurrent)
		}

		entryParent, err := getlsFileInfoEntry("", "..", true, flags)
		if err == nil && entryCurrent != entryParent {
			entries = append(entries, entryParent)
		}
	}

	// sort directory listing
	dir = sortDirs(dir, flags)

	for _, fileInfo := range dir {
		entry, err := getlsFileInfoEntry(fPath, fileInfo.Name(), false, flags)
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

	// print directory entries
	prettyPrintEntriesAdv(entries, cols, flags)

	// recursively print for sub-directories
	if flags.recursive {
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
	}
	return true // TODO: fix return value
}

func prettyPrintEntriesSimple(entries []lsFileInfo, sep string, flags flags) bool {
	for _, entry := range entries {
		lsInfo, _ := compileLsStringSimple(entry, flags)
		name := entry.name
		if flags.colors {
			name = colorFile(entry.name, (*entry.fileInfo).Mode())
		}
		if flags.classify {
			name = classifyFile(name, (*entry.fileInfo).Mode())
		}

		if len(lsInfo) > 0 {
			fmt.Printf("%s%s", lsInfo, flags.sep)
		}
		fmt.Printf("%s%s", name, sep)
	}
	return true
}

func prettyPrintEntriesAdv(entries []lsFileInfo, cols int, flags flags) bool {
	//fmt.Fprintf(os.Stderr, "cols:%d\n", cols)
	lfw := getlsFieldWidth(entries)

	// create output lines from entries
	entryStrings := make([]string, len(entries))
	lineLength := 0

	for i, entry := range entries {
		lsInfo, _ := compileLsStringSimple(entry, flags)
		entryStrings[i] = lsInfo
		lineLength += len(lsInfo) + len(entry.name) + len(flags.sep)
		if i+1 < len(entries) {
			lineLength += len(flags.sep)
		}
		if flags.classify {
			lineLength++
		}
	}

	//fmt.Fprintf(os.Stderr, "len:%d\n", length)

	// the output fits on one line, no need to use multiple entryStrings
	if lineLength < cols && flags.interactive {
		for i, entry := range entries {
			var name = entry.name
			if flags.colors {
				name = colorFile(entry.name, (*entry.fileInfo).Mode())
			}
			if flags.classify {
				name = classifyFile(name, (*entry.fileInfo).Mode())
			}
			if len(entryStrings[i]) > i+1 {
				fmt.Printf("%s%s", entryStrings[i], flags.sep)
			}
			fmt.Print(name)
			if i+1 < len(entries) {
				fmt.Print(flags.sep)
			}
		}
		fmt.Println()
		return true
	}

	if !flags.multiColumn {
		for _, entry := range entries {
			lsInfo, _ := compileLsString(entry, lfw, flags)
			name := entry.name
			if flags.colors {
				name = colorFile(entry.name, (*entry.fileInfo).Mode())
				if flags.classify {
					name = classifyFile(name, (*entry.fileInfo).Mode())
				}
			}
			if len(lsInfo) > 0 {
				fmt.Printf("%s%s", lsInfo, flags.sep)
			}
			fmt.Println(name)
		}
		return true
	}

	col := 0
	row := 0

	for _, entry := range entries {
		lsInfo, _ := compileLsString(entry, lfw, flags)

		var length int

		name := entry.name
		if flags.colors {
			name = colorFile(entry.name, (*entry.fileInfo).Mode())
		}
		if flags.classify {
			name = classifyFile(name, (*entry.fileInfo).Mode())
		}

		length += len(lsInfo) + len(entry.name)
		if len(lsInfo) > 0 {
			length += len(flags.sep)
		}

		if len(entry.name) > lfw.name {
			length += len(entry.name) - lfw.name
		}

		if col+length >= cols {
			col = 0
			row++
			fmt.Println()
		}

		if col > 0 {
			fmt.Print(flags.sep)
			col += len(flags.sep)
		}

		if flags.classify {
			length += 1
			col++
		}

		if len(lsInfo) > 0 {
			fmt.Printf("%s%s", lsInfo, flags.sep)
			col += len(lsInfo) + len(flags.sep)
		}
		fmt.Printf("%s", name)
		col += len(entry.name)

		// print spaces to justify output correctly
		for i := 0; i < lfw.name-len(entry.name) && col+1 < cols; i++ {
			fmt.Print(" ")
			col++
		}
	}

	if col > 0 {
		fmt.Println()
	}
	return true
}

// Parses command-line arguments in posix-style
// [-flags] [files...]
func parseArgs() (flags flags, args []string) {
	// initialize default config
	flags.base = 10
	flags.precision = 2
	//flags.sort = typeAlphaSort // see notice at the top of the file
	flags.sort = alphaSort // see notice at the top of the file
	flags.interactive = isTerminal()
	flags.multiColumn = flags.interactive
	//flags.colors = flags.interactive
	//flags.sep = "\t" // tab
	flags.sep = " " // tab
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
					//flags.linkCount = true
					flags.linkCount = true
					flags.size = true
					flags.group = true
					flags.owner = true
					flags.multiColumn = false
					flags.date = true

				case 'o': // like -l without group
					flags.permissions = true
					flags.linkCount = true
					flags.size = true
					flags.group = false
					flags.owner = true
					flags.multiColumn = false
					flags.date = true

					// ...
				case 'F': // reveal nature of a file
					flags.classify = true

				case 'f': // do not sort
					flags.sort = noSort

				case 'A': // list all files except . and ..
					flags.all = true
					flags.hideObvious = true

				case 'a': // list all files
					flags.all = true
					flags.hideObvious = false

				case 'r':
					flags.reverse = true

				case 'R': // recursively list
					flags.recursive = true

				case 'd': // info about file, not contents
					flags.nolisting = true

				case 't': // sort by mod time
					flags.sort = modTimeSort

				case 'g': // same as -l without owner
					flags.permissions = true
					flags.linkCount = true
					flags.size = true
					flags.group = true
					flags.owner = false
					flags.multiColumn = false
					flags.date = true

				case 'q':
					flags.nonGraphic = true

				case 'p':
					flags.revealDir = true
					fmt.Fprintf(os.Stderr, "WARNING: flag '-p' is not implemented!")

				case 'u':
					flags.sort = accessTimeSort

				case 'm':
					flags.sep = ","
					flags.multiColumn = true

				case 's': // size
					flags.size = true

				case 'S': // sort by size
					flags.sort = sizeSort

				case 'n': // numeric ids
					flags.numericIds = true
					flags.permissions = true
					flags.size = true
					flags.group = true
					flags.owner = true
					flags.multiColumn = false
					flags.date = true

				case 'i': // inode number
					flags.inode = true

				case 'h': // human
					flags.human = true

				case 'L':
					flags.dereferenceLink = true

				case 'H':
					fmt.Fprintf(os.Stderr, "WARNING: flag '-h' is not implemented!")

				case 'x': // multi-column output
					flags.multiColumn = true

				case '1': // one column per line
					flags.multiColumn = false

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
		args = append(args, os.Args[i])
	}

	if flags.human && !flags.size {
		fmt.Printf("Usage: %s [-1acdfFgGhilmnpqRsStux] [file ...]\n", os.Args[0])
		os.Exit(1)
	}

	if flags.multiColumn && flags.sep != "," {
		flags.sep = " "
	}

	return flags, args
}

func main() {
	flags, args := parseArgs()

	if len(args) == 0 {
		args = append(args, ".") // current directory
	}

	var dirs []string  // list of director paths to list
	var files []string // list of file paths to list

	for _, arg := range args {
		// clean path
		arg, _ = filepath.Abs(arg)
		arg = filepath.Clean(arg)
		finfo, err := os.Stat(arg)

		if err != nil {
			printError(err)
			continue
		}

		if finfo.IsDir() && !flags.nolisting {
			dirs = append(dirs, arg)
		} else {
			files = append(files, arg)
		}
	}

	// print single files
	if len(files) > 0 {
		cols, err := getTerminalWidth()
		if err != nil {
			cols = 80 // default cols
		}

		var entries []lsFileInfo

		// cwd, err := os.Getwd()
		// if err != nil {
		// 	printError(err)
		// 	os.Exit(1)
		// 	return
		// }

		for _, filePath := range files {
			info, err := getlsFileInfoEntry("", filePath, true, flags)
			if err != nil {
				printError(err)
				continue
			}
			entries = append(entries, info)
		}
		prettyPrintEntriesAdv(entries, cols, flags)

		// separate list of single files from directories
		if len(files) > 0 {
			fmt.Println()
			if flags.multiColumn {
				fmt.Println()
			}
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

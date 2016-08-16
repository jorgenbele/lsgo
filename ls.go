// A implementation of the 'ls' unix utility in go, using 'some' cgo
// TODO: pretty print multi columns, support windows(?) + other os's, show
// symbolic links (required in various scripts, f.ex. the libre office
// launcher), remove Cgo dependencies

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

	"runtime"
	"syscall"
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
	AlphaSort                  // alpha sort
	SizeSort                   // numeric sort on file size
	ModTimeSort                // last-modification sort
	AccessTimeSort             // last-access sort
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

	permissions bool // print permissions
	size        bool // -s - file size
	recursive   bool // -R - recursively list for subdirectories
	all         bool // -a or -A - show all files

	group bool // print owner of file
	owner bool // print group of file

	inode       bool // -i - show inode number
	numeric_ids bool // -n - print owner/group as uid and gid instead of name

	non_graphic bool // -q - force print non graphic characters to screen
	reveal_dir  bool // -p - add / to end of directory names (as in -F)
}

var Args []string

func printError(err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
}

// Returns the size of a file (binary size) in a
// format easily read by humans.
func sizeToHuman(size int64, base, precision uint) string {
	sizes := []string{"", "K", "M", "G", "T", "P", "E"}

	fsize := float64(size)
	var i int64 = 0

	for ; fsize > 1024; i++ {
		fsize /= 1024
	}

	if i != 0 && i < int64(len(sizes)) {
		return strconv.FormatFloat(fsize, 'f', int(precision), 64) + sizes[i]
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

		// executable
	} else if mode.Perm()&0 != 0 {
		return filepath.Clean(fPath) + "*"
	}

	return fPath
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

	// retrive additional information about the file, UNSAFE
	stat, ownername, groupname, _ := getFileStat(fPath, flags)

	if flags.inode {
		info += strconv.FormatUint(uint64(stat.Ino), int(flags.base)) + flags.sep
	}

	if flags.permissions {
		perm := (*file).Mode().String()
		info += perm + flags.sep
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
			// if printing multiple entries per line, do not include uneccesary whitespace
			if flags.multi_column {
				info += fmt.Sprintf("%s",
					sizeToHuman(size, flags.base, flags.precision))
			} else {
				// the 'size' field with will always be at most 4 characters
				// wide + an extra character (K, M, G etc), if not considering
				// the precision (according to sizeToHuman)
				info += fmt.Sprintf("%*s", int(flags.precision+4+1),
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

	return info + relativePath, nil
}

// sort interfaces
type SortByAlpha []os.FileInfo

func (a SortByAlpha) Len() int           { return len(a) }
func (a SortByAlpha) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortByAlpha) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

type SortByModTime []os.FileInfo

func (a SortByModTime) Len() int           { return len(a) }
func (a SortByModTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortByModTime) Less(i, j int) bool { return a[i].ModTime().Before(a[j].ModTime()) }

type SortByAccessTime []os.FileInfo

func (a SortByAccessTime) Len() int      { return len(a) }
func (a SortByAccessTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortByAccessTime) Less(i, j int) bool {
	accessTime := func(fPath string) C.time_t {
		return C.access_time(C.CString(fPath))
	}
	return accessTime(a[i].Name()) < accessTime(a[i].Name())
}

type SortBySize []os.FileInfo

func (a SortBySize) Len() int           { return len(a) }
func (a SortBySize) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortBySize) Less(i, j int) bool { return a[i].Size() < a[j].Size() }

// Sorts the []os.FileInfo slice by flags
func sortDirs(dir []os.FileInfo, flags Flags) []os.FileInfo {

	switch flags.sort {
	case AlphaSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(SortByAlpha(dir)))
		} else {
			sort.Sort(SortByAlpha(dir))
		}

	case ModTimeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(SortByModTime(dir)))
		} else {
			sort.Sort(SortByModTime(dir))
		}
	case AccessTimeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(SortByAccessTime(dir)))
		} else {
			sort.Sort(SortByAccessTime(dir))
		}

	case SizeSort:
		if flags.reverse {
			sort.Sort(sort.Reverse(SortBySize(dir)))
		} else {
			sort.Sort(SortBySize(dir))
		}
	}
	return dir
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
		col += len(s) + len(flags.sep) // length + space flags.sep

		if col <= cols && flags.multi_column {
			fmt.Printf("%s%s", s, flags.sep)
		} else {
			if printed > 0 {
				fmt.Println()
			}
			col = len(s) + len(flags.sep) // length + space flags.sep
			fmt.Printf("%s%s", s, flags.sep)
		}
		printed++
	}

	// print files in directory
	for _, file := range dir {
		var cPath string

		if filepath.IsAbs(file.Name()) {
			cPath = filepath.Clean(file.Name())
		} else {
			cPath = filepath.Join(fPath, file.Name())
		}

		info, err := getFileInfo(cPath, &file, flags)

		if err != nil {
			continue
		}

		printNext(info)
		paths = append(paths, cPath)
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

			if err != nil {
				printError(err)
				dir.Close()
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
			dir.Close()
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
	flags.sort = AlphaSort
	flags.interactive = isTerminal()
	flags.multi_column = flags.interactive
	flags.sep = "\t" // tab

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
					flags.size = true
					flags.group = true
					flags.owner = true
					flags.multi_column = false

					// ...
				case 'F': // reveal nature of a file
					flags.classify = true

				case 'f': // do not sort
					flags.sort = NoSort
					fallthrough
				case 'a': // list all files
					fallthrough
				case 'A':
					flags.all = true

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

				case 'o':
					// not implemented...
					//flags.owner = true
					break

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

	for i, arg := range Args {
		// clean path
		arg = filepath.Clean(arg)

		file, err := os.Open(arg)
		defer file.Close()

		if err != nil {
			printError(err)
			continue
		}

		finfo, err := file.Stat()

		if err != nil {
			printError(err)
			continue
		}

		if len(Args) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("%s:\n", arg)
		}

		if finfo.IsDir() {

			dir, err := file.Readdir(0)

			if err != nil {
				continue
			}

			printDirEntries(arg, dir, flags)
		} else {
			info, _ := getFileInfo(arg, &finfo, flags)
			fmt.Println(info)
		}
	}
}

// Flags, a package used in the argument parsing used in the 'ls' utility

// Author: Jorgen Bele <jorgen.bele@gmail.com>, 2016
// Date:   14.03.2016 (dd.mm.yyyy)

package flags

import (
	"fmt"
	"os"
)

type Sort int

const (
	NoSort      Sort = iota // don't sort
	NumericSort             // numeric sort
	AlphaSort               // alpha sort
	SizeSort                // numeric sort on file size
	ModTimeSort             // last-modification sort
)

type Flags struct {
	// printing settings
	Base      uint // numeric base (only for ints)
	Precision uint // numeric precision
	Human     bool // -h - human readable
	Classify  bool // -F - append character revealing filetype
	Nolisting bool // -d - shows info about file, but not its contents

	// sort
	Sort    Sort // -f - sort type
	Reverse bool // -r - list files in reverse

	// compatibility
	Colors      bool // -G - use ansi colors
	Interactive bool // interactive or piped

	Permissions bool // print permissions
	Size        bool // -s - file size
	Recursive   bool // -R - recursively list for subdirectories
	All         bool // -a or -A - show all files

	Group bool // -g - print owner of file
	Owner bool // -o - print group of file

	Inode       bool // -i - show inode number
	Numeric_ids bool // -n - print owner/group as uid and gid instead of name
}

var Args []string

// Parses command-line arguments in posix-style
// [-flags] [files...]
func ParseFlags() Flags {
	// initialize default config
	var flags Flags
	flags.Base = 10
	flags.Precision = 2
	flags.Sort = AlphaSort

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
					flags.Inode = true
					flags.Permissions = true
					flags.Size = true
					flags.Group = true
					flags.Owner = true

					// ...
				case 'f': // do not sort
					flags.Sort = NoSort
				case 'F': // reveal nature of a file
					flags.Classify = true

				case 'a': // list all files
					fallthrough
				case 'A':
					flags.All = true

				case 'r':
					flags.Reverse = true
				case 'R': // recursively list
					flags.Recursive = true
				case 'd': // info about file, not contents
					flags.Nolisting = true
				case 't': // sort by mod time
					flags.Sort = ModTimeSort

				case 'g':
					flags.Group = true
				case 'o':
					flags.Owner = true

				case 's': // size
					flags.Size = true

				case 'S': // sort by size
					flags.Sort = SizeSort

				case 'n': // numeric ids
					flags.Numeric_ids = true

				case 'i': // Inode number
					flags.Inode = true

				case 'h': // human
					flags.Human = true
				case 'G': // colors
					flags.Colors = true

				default:
					fmt.Fprintf(os.Stderr, "%s: unknown flag %q, ignoring\n", os.Args[0], c)
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

	if flags.Human && !flags.Size {
		fmt.Printf("Usage: %s [-lfFaArRdtgosSnihG] [file ...]\n", os.Args[0])
		os.Exit(1)
	}

	return flags
}

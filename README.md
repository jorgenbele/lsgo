# lsgo
An attempt at writing the standard *ls* UNIX utility in Go.

## Installation
```
go get jorgenbele/lsgo
```

## Usage
```
Usage: lsgo [-1acdfFgGhilmnpqRsStux] [file ...]
-a  list all files
-A  list all files except . and ..
-d  list information about the directory itself, not its contents
-f  disable sorting of entries
-F  classify file type by appending */=@| to entry filenames
-g  -l, without owner
-G  enable colors (NOTE: not standard)
-h  enable human output (also prints the help message if no other flags are enabled)
-H  not implemented
-i  display inode numbers
-l  use long listing format
-L  dereference links
-m  list output in columns separated by commas (',')
-n  display numeric ids
-o  -l, without group information
-p  reveal directories by appending a '/' character
-q  replace all nongraphic characters with ?
-r  reverse output
-R  recursive listing of subdirectories
-s  display file size information
-S  sort by file size
-t  sort output by modification time
-u  sort output by access time
-x  enable output of multi-entry per line
-1  force output of one entry per line
```

## Motivation
To learn Go.

## Issues
* Limited color support. Currently uses hardcoded colors. 
* Several features (flags) are not implemented.
* Not all flags are standard.

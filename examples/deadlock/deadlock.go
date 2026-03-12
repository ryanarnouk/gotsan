package main

import (
	"path"
	"sync"
)

// File represents a file on cloud storage
type File struct {
	mu sync.Mutex
	// @guarded_by(mu)
	name string
	// @guarded_by(mu)
	size int64
	// @guarded_by(mu)
	parent *Dir
}

// Path returns the full path of the File object
//
// @acquires(mu)
func (f *File) Path() string {
	f.mu.Lock()
	name := f.name
	parent := f.parent
	f.mu.Unlock()
	return path.Join(parent.Path(), name)
}

// Size of the file object
//
// @acquires(mu)
func (f *File) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.size
}

// Truncate the file to size
//
// @acquires(mu)
func (f *File) Truncate(size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.size = size
}

// Dir represents a directory on cloud storage
type Dir struct {
	mu sync.Mutex
	// @guarded_by(mu)
	name string
	// @guarded_by(mu)
	parent *Dir
	// @guarded_by(mu)
	files []*File
	// @guarded_by(mu)
	dirs []*Dir
}

// Path returns the full path of the Dir
//
// @acquires(mu)
func (d *Dir) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.parent == nil {
		return "/"
	}
	return path.Join(d.parent.Path(), d.name)
}

// Size of all the objects in the Dir
//
// @acquires(mu)
func (d *Dir) Size() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	var total int64
	for _, f := range d.files {
		total += f.Size()
	}
	return total
}

// process1 repeatedly traverses file path state.
func process1(f *File) {
	for {
		f.Path()
	}
}

// process2 repeatedly computes directory size.
func process2(d *Dir) {
	for {
		d.Size()
	}
}

func main() {
	root := &Dir{}
	var mu *sync.Mutex
	//@supress_guarded_by
	file1 := &File{mu: *mu, name: "file1", size: 42, parent: root}
	root.files = []*File{file1}

	go process1(file1)
	go process2(root)

	select {}
}

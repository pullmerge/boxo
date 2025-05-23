package filestore

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"testing"

	blockstore "github.com/ipfs/boxo/blockstore"
	posinfo "github.com/ipfs/boxo/filestore/posinfo"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ipld "github.com/ipfs/go-ipld-format"
)

var bg = context.Background()

func newTestFilestore(t *testing.T, option ...Option) (string, *Filestore) {
	mds := ds.NewMapDatastore()

	testdir := t.TempDir()
	fm := NewFileManager(mds, testdir, option...)
	fm.AllowFiles = true

	bs := blockstore.NewBlockstore(mds)
	fstore := NewFilestore(bs, fm)
	return testdir, fstore
}

func makeFile(t *testing.T, dir string, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "file")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.Close()
	})

	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	return f.Name()
}

func TestBasicFilestore(t *testing.T) {
	cases := []struct {
		name    string
		options []Option
	}{
		{"default", nil},
		{"mmap", []Option{WithMMapReader()}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, fs := newTestFilestore(t, c.options...)

			buf := make([]byte, 1000)
			rand.Read(buf)

			fname := makeFile(t, dir, buf)

			var cids []cid.Cid
			for i := 0; i < 100; i++ {
				n := &posinfo.FilestoreNode{
					PosInfo: &posinfo.PosInfo{
						FullPath: fname,
						Offset:   uint64(i * 10),
					},
					Node: dag.NewRawNode(buf[i*10 : (i+1)*10]),
				}

				err := fs.Put(bg, n)
				if err != nil {
					t.Fatal(err)
				}
				cids = append(cids, n.Node.Cid())
			}

			for i, c := range cids {
				blk, err := fs.Get(bg, c)
				if err != nil {
					t.Fatal(err)
				}

				if !bytes.Equal(blk.RawData(), buf[i*10:(i+1)*10]) {
					t.Fatal("data didnt match on the way out")
				}
			}

			kch, err := fs.AllKeysChan(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			out := make(map[string]struct{})
			for c := range kch {
				out[c.KeyString()] = struct{}{}
			}

			if len(out) != len(cids) {
				t.Fatal("mismatch in number of entries")
			}

			for _, c := range cids {
				if _, ok := out[c.KeyString()]; !ok {
					t.Fatal("missing cid: ", c)
				}
			}
		})
	}
}

func randomFileAdd(t *testing.T, fs *Filestore, dir string, size int) (string, []cid.Cid) {
	buf := make([]byte, size)
	rand.Read(buf)

	fname := makeFile(t, dir, buf)

	var out []cid.Cid
	for i := 0; i < size/10; i++ {
		n := &posinfo.FilestoreNode{
			PosInfo: &posinfo.PosInfo{
				FullPath: fname,
				Offset:   uint64(i * 10),
			},
			Node: dag.NewRawNode(buf[i*10 : (i+1)*10]),
		}
		err := fs.Put(bg, n)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n.Cid())
	}

	return fname, out
}

func TestDeletes(t *testing.T) {
	dir, fs := newTestFilestore(t)
	_, cids := randomFileAdd(t, fs, dir, 100)
	todelete := cids[:4]
	for _, c := range todelete {
		err := fs.DeleteBlock(bg, c)
		if err != nil {
			t.Fatal(err)
		}
	}

	deleted := make(map[string]bool)
	for _, c := range todelete {
		_, err := fs.Get(bg, c)
		if !ipld.IsNotFound(err) {
			t.Fatal("expected blockstore not found error")
		}
		deleted[c.KeyString()] = true
	}

	keys, err := fs.AllKeysChan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for c := range keys {
		if deleted[c.KeyString()] {
			t.Fatal("shouldnt have reference to this key anymore")
		}
	}
}

func TestIsURL(t *testing.T) {
	if !IsURL("http://www.example.com") {
		t.Fatal("IsURL failed: http://www.example.com")
	}
	if !IsURL("https://www.example.com") {
		t.Fatal("IsURL failed: https://www.example.com")
	}
	if IsURL("adir/afile") || IsURL("http:/ /afile") || IsURL("http:/a/file") {
		t.Fatal("IsURL recognized non-url")
	}
}

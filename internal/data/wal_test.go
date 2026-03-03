package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/vmihailenco/msgpack/v5"
)

func TestNewWAL_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("WAL file not created at %s", path)
	}
}

func TestWALAppend_And_Replay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ws := Workspace{
		ID:   ksuid.New(),
		Name: "test-workspace",
	}
	wsData, err := msgpack.Marshal(&ws)
	if err != nil {
		t.Fatalf("msgpack.Marshal workspace: %v", err)
	}

	if err := w.Append(OpAddWorkspace, ws.ID, wsData); err != nil {
		t.Fatalf("Append: %v", err)
	}
	w.Close()

	// Replay into a fresh Data.
	w2, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL (replay): %v", err)
	}
	defer w2.Close()

	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
		Jobs:       make(map[ksuid.KSUID]*Job),
	}

	if err := w2.Replay(db); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	got, ok := db.Workspaces[ws.ID]
	if !ok {
		t.Fatalf("workspace not found after replay")
	}
	if got.Name != ws.Name {
		t.Errorf("Name = %q, want %q", got.Name, ws.Name)
	}
}

func TestWALAppend_RemoveWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ws := Workspace{
		ID:   ksuid.New(),
		Name: "to-be-removed",
	}
	wsData, _ := msgpack.Marshal(&ws)

	_ = w.Append(OpAddWorkspace, ws.ID, wsData)
	_ = w.Append(OpRemoveWorkspace, ws.ID, nil)
	w.Close()

	w2, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL (replay): %v", err)
	}
	defer w2.Close()

	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
		Jobs:       make(map[ksuid.KSUID]*Job),
	}

	if err := w2.Replay(db); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	if _, ok := db.Workspaces[ws.ID]; ok {
		t.Error("workspace still present after remove replay")
	}
}

func TestWALAppend_Catalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	cat := Catalog{
		ID:   ksuid.New(),
		Name: "my-catalog",
	}
	catData, _ := msgpack.Marshal(&cat)

	_ = w.Append(OpAddCatalog, cat.ID, catData)
	w.Close()

	w2, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL (replay): %v", err)
	}
	defer w2.Close()

	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
		Jobs:       make(map[ksuid.KSUID]*Job),
	}

	if err := w2.Replay(db); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	if _, ok := db.Catalogs[cat.ID]; !ok {
		t.Error("catalog not found after replay")
	}
}

func TestWALTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}

	ws := Workspace{ID: ksuid.New(), Name: "truncate-test"}
	wsData, _ := msgpack.Marshal(&ws)
	_ = w.Append(OpAddWorkspace, ws.ID, wsData)

	if err := w.Truncate(); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	// File should exist but be empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat after truncate: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("WAL size after truncate = %d, want 0", info.Size())
	}

	// Replay should not restore the workspace.
	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
		Jobs:       make(map[ksuid.KSUID]*Job),
	}
	if err := w.Replay(db); err != nil {
		t.Fatalf("Replay after truncate: %v", err)
	}
	if len(db.Workspaces) != 0 {
		t.Errorf("expected empty workspaces after truncate, got %d", len(db.Workspaces))
	}

	w.Close()
}

func TestWALReplay_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.wal")

	w, err := NewWAL(path)
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}
	defer w.Close()

	db := &Data{
		Workspaces: make(map[ksuid.KSUID]*Workspace),
		Catalogs:   make(map[ksuid.KSUID]*Catalog),
		Threads:    make(map[ksuid.KSUID]*Thread),
		Pipelines:  make(map[ksuid.KSUID]*Pipeline),
		Jobs:       make(map[ksuid.KSUID]*Job),
	}

	if err := w.Replay(db); err != nil {
		t.Fatalf("Replay on empty WAL: %v", err)
	}
	if len(db.Workspaces) != 0 {
		t.Errorf("expected empty workspaces, got %d", len(db.Workspaces))
	}
}

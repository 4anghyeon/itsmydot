package tui

import (
	"context"
	"os"
	"path"

	"github.com/4anghyeon/itsmydot/internal/diff"
	"github.com/4anghyeon/itsmydot/internal/github"
	"github.com/4anghyeon/itsmydot/internal/link"
	"github.com/4anghyeon/itsmydot/internal/localcopy"
	"github.com/4anghyeon/itsmydot/internal/manifest"
)

// entriesLoadedMsg 는 manifest를 불러와 각 항목의 상태까지 확인한 결과다.
type entriesLoadedMsg struct {
	items  []item
	hashes map[string]string // 저장소 파일의 blob 해시. 받지 못했으면 nil이다
	err    error
}

// entrySyncedMsg 는 syncEntry나 fetchCopy가 항목 하나를 받아 온 결과다.
type entrySyncedMsg struct {
	name        string
	backup      string // 기존 파일을 옮겨 둔 백업 경로. 백업하지 않았으면 빈 문자열
	copyUpdated bool   // 연결은 그대로 두고 로컬 사본만 새로 받았으면 true
	err         error
}

// diffLoadedMsg 는 저장소 최신본과 비교한 결과다. 두 파일이 같으면 hunks가 비어 있다.
type diffLoadedMsg struct {
	name    string
	hunks   []diff.Hunk
	err     error
	current bool // 로컬 사본 대신 target 자리의 지금 파일과 비교했으면 true. 충돌 질문에서 연 비교다
}

// previewLoadedMsg 는 미리보기 창에 보여 줄 저장소 파일을 받아 온 결과다. key는 캐시에 넣을 정리한 source 경로다.
type previewLoadedMsg struct {
	name string
	key  string
	data []byte
	err  error
}

// loadEntries 는 GitHub에서 manifest를 받아 파싱하고 각 항목의 현재 상태를 확인한다. 디스크는 바꾸지 않는다.
func loadEntries(ctx context.Context, client *github.Client, copies *localcopy.Dir) entriesLoadedMsg {
	data, err := client.Fetch(ctx, "manifest.yaml")
	if err != nil {
		return entriesLoadedMsg{err: err}
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return entriesLoadedMsg{err: err}
	}

	hashes, _ := client.FileHashes(ctx)

	// 하위 항목은 그룹 바로 뒤에 이어 붙여서 목록 순서가 화면 순서와 같게 한다.
	var items []item
	for _, e := range m.Entries {
		items = append(items, checkItem(e, "", copies, hashes))
		for _, child := range e.Children {
			items = append(items, checkItem(child, e.Name, copies, hashes))
		}
	}
	return entriesLoadedMsg{items: items, hashes: hashes}
}

// checkItem 은 항목 하나의 로컬 사본 경로와 지금 상태를 확인해 목록의 한 줄을 만든다.
// 확인에 실패해도 나머지 항목은 보여 주도록 그 항목만 errored로 표시한다.
func checkItem(e manifest.Entry, parent string, copies *localcopy.Dir, hashes map[string]string) item {
	e.Children = nil
	it := item{entry: e, parent: parent}
	var err error
	it.copyPath, err = copies.Path(e.Source)
	if err == nil {
		var s link.State
		s, err = link.Check(e.Target, it.copyPath)
		it.state = fromLinkState(s)
	}
	if err != nil {
		it.state, it.err = errored, err
	}
	if it.state == synced {
		it.outdated = differsFromRepo(it.copyPath, e.Source, hashes)
	}
	return it
}

// differsFromRepo 는 로컬 사본이 저장소의 source 파일과 다른지 알려 준다.
// 저장소 해시가 없거나 사본을 읽지 못하면 비교할 수 없으므로 다르지 않다고 본다.
func differsFromRepo(copyPath, source string, hashes map[string]string) bool {
	remote, ok := hashes[path.Clean(source)]
	if !ok {
		return false
	}
	data, err := os.ReadFile(copyPath)
	if err != nil {
		return false
	}
	return github.BlobHash(data) != remote
}

// syncEntry 는 항목 하나를 GitHub에서 최신본으로 받아 로컬 사본에 저장하고 target에 연결한다.
// 이미 연결된 항목이면 로컬 사본만 최신으로 바뀐다.
func syncEntry(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name}

	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	copyPath, err := copies.Write(e.Source, data)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.backup, msg.err = link.Link(e.Target, copyPath)
	return msg
}

// fetchCopy 는 연결된 항목의 최신본을 받아 로컬 사본만 바꾼다. symlink는 사본을 가리키고 있으므로 바로 반영된다.
func fetchCopy(ctx context.Context, client *github.Client, copies *localcopy.Dir, e manifest.Entry) entrySyncedMsg {
	msg := entrySyncedMsg{name: e.Name, copyUpdated: true}
	data, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	_, msg.err = copies.Write(e.Source, data)
	return msg
}

// fromLinkState 는 link 패키지의 디스크 상태를 화면 상태로 바꾼다.
func fromLinkState(s link.State) itemState {
	switch s {
	case link.Synced:
		return synced
	case link.Conflict:
		return conflict
	default:
		return notSynced
	}
}

// compareWithRepo 는 저장소 최신본을 받아 path의 파일과 비교한다. 디스크는 바꾸지 않는다.
func compareWithRepo(ctx context.Context, client *github.Client, e manifest.Entry, path string) diffLoadedMsg {
	msg := diffLoadedMsg{name: e.Name}
	remote, err := client.Fetch(ctx, e.Source)
	if err != nil {
		msg.err = err
		return msg
	}
	local, err := os.ReadFile(path)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.hunks, msg.err = diff.Compare(local, remote)
	return msg
}

package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"unicode/utf8"
)

// textLimit 은 read 하는 파일과 write 하는 text 의 최대 바이트 수다.
const textLimit = 8388608

// bytesLimit is the largest file that readBytes reads and writeBytes writes; its base64 stays within a request line.
const bytesLimit = 33554432

// byteOrderMark 는 UTF-8 의 BOM 이다.
var byteOrderMark = []byte{0xef, 0xbb, 0xbf}

func tooLarge(size, limit int64, path string) error {
	return fmt.Errorf("file is %d bytes, above the %d-byte limit: %s", size, limit, path)
}

func version(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// readFile returns the content of the regular file path inside root when it has at most limit bytes.
func readFile(root, path string, limit int64) ([]byte, error) {
	target, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	// 크기와 종류는 열기 전에 확인한다. FIFO 는 읽으려고 열면 멈춘다.
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	if info.Size() > limit {
		return nil, tooLarge(info.Size(), limit, path)
	}
	return os.ReadFile(target)
}

// ReadBytes puts the content of the regular file path inside root into body as base64 with its version.
func ReadBytes(root, path string, body *EventBody) error {
	content, err := readFile(root, path, bytesLimit)
	if err != nil {
		return err
	}
	data := base64.StdEncoding.EncodeToString(content)
	body.Data = &data
	body.Version = version(content)
	return nil
}

// Read 는 root 안의 정규 파일 path 의 내용을 text, 판, 줄바꿈, BOM 으로 body 에 담는다.
func Read(root, path string, body *EventBody) error {
	content, err := readFile(root, path, textLimit)
	if err != nil {
		return err
	}
	text, bom := bytes.CutPrefix(content, byteOrderMark)
	if !utf8.Valid(text) {
		return fmt.Errorf("not UTF-8 text: %s", path)
	}
	value := string(text)
	body.Text = &value
	body.Version = version(content)
	body.Newline = newline(text)
	body.BOM = &bom
	return nil
}

// newline 은 text 의 줄바꿈 종류다. 한 종류만 있으면 그 이름, 여러 종류가 있으면 mixed, 없으면 none 이다.
func newline(text []byte) string {
	var lf, crlf, cr bool
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\r' && i+1 < len(text) && text[i+1] == '\n':
			crlf = true
			i++
		case text[i] == '\r':
			cr = true
		case text[i] == '\n':
			lf = true
		}
	}
	kinds := 0
	name := "none"
	for _, each := range []struct {
		found bool
		name  string
	}{{lf, "lf"}, {crlf, "crlf"}, {cr, "cr"}} {
		if each.found {
			kinds++
			name = each.name
		}
	}
	if kinds > 1 {
		return "mixed"
	}
	return name
}

// requestExpect reads the expect field of a write request of operation: a version or null. A request without the
// field is refused instead of being taken as null.
func requestExpect(request Request, operation string) (*string, error) {
	if len(request.Body.Expect) == 0 {
		return nil, fmt.Errorf("%s requires expect: a version or null", operation)
	}
	var expect *string
	if err := json.Unmarshal(request.Body.Expect, &expect); err != nil {
		return nil, fmt.Errorf("%s expect %s is not a version or null", operation, request.Body.Expect)
	}
	if expect != nil {
		if decoded, err := hex.DecodeString(*expect); err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != *expect {
			return nil, fmt.Errorf("%s expect %q is not a SHA-256 version", operation, *expect)
		}
	}
	return expect, nil
}

// writeBytesRequest checks the fields of a writeBytes request and writes the decoded bytes.
func writeBytesRequest(request Request, body *EventBody) error {
	if request.Body.Data == nil {
		return errors.New("writeBytes requires data")
	}
	expect, err := requestExpect(request, "writeBytes")
	if err != nil {
		return err
	}
	content, err := base64.StdEncoding.Strict().DecodeString(*request.Body.Data)
	if err != nil {
		return errors.New("writeBytes data is not base64")
	}
	if len(content) > bytesLimit {
		return tooLarge(int64(len(content)), bytesLimit, request.Body.Path)
	}
	written, err := writeFile(request.Root, request.Body.Path, content, expect)
	if err != nil {
		return err
	}
	body.Version = written
	return nil
}

// writeRequest 는 write 요청의 필드를 확인하고 Write 를 실행한다. expect 가 빠진 요청은 null 로 보지 않고 거절한다.
func writeRequest(request Request, body *EventBody) error {
	if request.Body.Text == nil {
		return errors.New("write requires text")
	}
	if request.Body.BOM == nil {
		return errors.New("write requires bom")
	}
	expect, err := requestExpect(request, "write")
	if err != nil {
		return err
	}
	written, err := Write(request.Root, request.Body.Path, *request.Body.Text, expect, *request.Body.BOM)
	if err != nil {
		return err
	}
	body.Version = written
	return nil
}

// Write 는 root 안의 path 에 text 를, bom 이면 BOM 뒤에 쓰고 쓴 바이트의 판을 반환한다.
// expect 가 판이면 있는 정규 파일을 자르지 않고 열어 같은 기술자로 읽은 판이 expect 와 같을 때만 처음부터 덮어쓰고
// 쓴 길이로 자른다. 그래서 inode, 권한, 확장 속성, ACL, 하드 링크, 심볼릭 링크가 그대로 남는다.
// expect 가 nil 이면 새 파일을 0666(umask 적용)으로 만들고, 경로가 있으면 실패한다.
func Write(root, path, text string, expect *string, bom bool) (string, error) {
	if len(text) > textLimit {
		return "", tooLarge(int64(len(text)), textLimit, path)
	}
	content := []byte(text)
	if bom {
		content = append(append([]byte{}, byteOrderMark...), content...)
	}
	return writeFile(root, path, content, expect)
}

// writeFile writes content to path inside root with the in-place, expect and creation rules of Write and returns
// the version of content.
func writeFile(root, path string, content []byte, expect *string) (string, error) {
	file, err := openForWrite(root, path, expect)
	if err != nil {
		return "", err
	}
	written, err := file.WriteAt(content, 0)
	if err == nil {
		err = file.Truncate(int64(written))
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil && written > 0 {
		return "", fmt.Errorf("write incomplete: %s: %w", path, err)
	}
	if err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return version(content), nil
}

// openForWrite 는 Write 가 쓸 파일을 연다. expect 가 판이면 있는 파일의 판을 확인한 뒤 반환한다.
func openForWrite(root, path string, expect *string) (*os.File, error) {
	if expect == nil {
		target, err := resolveIn(root, path, true)
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("exists: %s", path)
		}
		return file, err
	}
	target, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	// 종류는 열기 전에 확인한다. FIFO 는 열면 멈출 수 있다.
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	// 판을 같은 기술자로 읽으려고 O_WRONLY 대신 O_RDWR 로 열고, 자르지 않는다.
	file, err := os.OpenFile(target, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, errors.Join(fmt.Errorf("read %s: %w", path, err), file.Close())
	}
	if hex.EncodeToString(hash.Sum(nil)) != *expect {
		return nil, errors.Join(fmt.Errorf("changed on disk: %s", path), file.Close())
	}
	return file, nil
}

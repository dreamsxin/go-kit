package tools_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestGeneratedCRUDAgainstSQLite is the acceptance gate for the generated
// database path: it generates a project from a real schema, fills the five
// stubs microgen deliberately leaves in the user-owned service file, builds it,
// runs it as a process, and drives a full CRUD cycle over HTTP.
//
// Nothing else covers this composition. Every other generated-project test runs
// with -db=false and starts the binary only to read /health, so the generated
// gorm repository, the generated pagination window, and the errors.Is branches
// that turn a unique-constraint violation into 409 have never run together.
// Each of those is a promise the generator makes in code it writes; the only
// way to keep the promise honest is to execute it.
//
// The service bodies are filled here rather than generated because microgen
// writes them once and never overwrites them — business logic is the user's.
// What the fixture writes is the plainest possible implementation on top of the
// generated seam, so a failure names the framework rather than the fixture.
//
// The generated main imports gorm.io/driver/sqlite, which needs cgo. The
// Windows verify job deliberately installs no mingw toolchain, so this gate
// skips there for the same reason the protobuf gate skips without protoc:
// report the missing capability instead of failing on it.
func TestGeneratedCRUDAgainstSQLite(t *testing.T) {
	t.Parallel()
	requireCToolchain(t)

	root := goKitRoot(t)
	dir := generatedProjectDir(t, "gen_fromdb_crud")
	const importPath = "example.com/gen_fromdb_crud"

	// The schema the generator introspects is not the database the service
	// runs on: introspection reads a file with data in it, while the running
	// service builds its table from the generated model via -auto-migrate.
	// Keeping them separate is what makes the second half a real assertion.
	schemaPath := filepath.Join(t.TempDir(), "schema.sqlite")
	createSQLiteSchema(t, schemaPath)
	runPath := filepath.Join(t.TempDir(), "app.sqlite")

	if out, err := microgenCommand(t,
		"-from-db",
		"-driver", "sqlite",
		"-dsn", schemaPath,
		"-service", "CatalogService",
		"-out", dir,
		"-import", importPath,
		"-db",
		"-resource",
		"-config=false",
		"-docs=false",
		"-openapi=false",
	).CombinedOutput(); err != nil {
		t.Fatalf("microgen -from-db: %v\n%s", err, out)
	}

	installGeneratedCRUDBodies(t, filepath.Join(dir, "service", "catalogservice", "service.go"), importPath)
	pinGeneratedModule(t, dir, root)

	binName := "gen_fromdb_crud_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)
	build := exec.Command("go", "build", "-mod=mod", "-o", binPath, "./cmd")
	build.Dir = dir
	if out := runCommand(t, build); out != "" {
		t.Logf("build output: %s", out)
	}

	addr := freeTCPAddr(t)
	baseURL := "http://" + addr
	serve := exec.Command(binPath, "-http.addr="+addr, "-db.dsn="+runPath, "-auto-migrate")
	serve.Dir = dir
	serve.Env = os.Environ()
	// The process output is captured because a startup failure is otherwise
	// reported as "the port never answered", which names the symptom and hides
	// the cause.
	var serveOutput bytes.Buffer
	serve.Stdout = &serveOutput
	serve.Stderr = &serveOutput
	if err := serve.Start(); err != nil {
		t.Fatalf("start generated project: %v", err)
	}
	defer killCmd(t, serve)
	exited := make(chan error, 1)
	go func() { exited <- serve.Wait() }()
	startupBegan := time.Now()
	waitForGeneratedService(t, baseURL+"/health", exited, &serveOutput)
	t.Logf("generated service answered /health in %s", time.Since(startupBegan).Round(10*time.Millisecond))

	// 1. Create. The response is the generated DTO, not the model: a field
	// missing here means the generator's mapping dropped it.
	created := callGeneratedAPI(t, http.MethodPost, baseURL+"/user", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
	})
	if created.status != http.StatusOK {
		t.Fatalf("POST /user: status %d, body %s", created.status, created.raw)
	}
	id := jsonInt(t, created.body, "data", "id")
	if got := jsonString(t, created.body, "data", "email"); got != "alice@example.com" {
		t.Errorf("POST /user: data.email = %q, want alice@example.com", got)
	}

	// 2. Read back. This is the only assertion that proves the write reached
	// the database rather than a request-scoped value.
	read := callGeneratedAPI(t, http.MethodGet, baseURL+"/user/"+strconv.Itoa(id), nil)
	if read.status != http.StatusOK {
		t.Fatalf("GET /user/%d: status %d, body %s", id, read.status, read.raw)
	}
	if got := jsonString(t, read.body, "data", "username"); got != "alice" {
		t.Errorf("GET /user/%d: data.username = %q, want alice", id, got)
	}

	// 3. The unique constraint. The driver rejects the insert, the generated
	// repository classifies it as ErrDuplicated, and the service maps that to
	// KindAlreadyExists — three separate layers agreeing on 409. Before this
	// test, the only thing checking the middle layer was a comment.
	duplicate := callGeneratedAPI(t, http.MethodPost, baseURL+"/user", map[string]any{
		"username": "alice-again",
		"email":    "alice@example.com",
	})
	if duplicate.status != http.StatusConflict {
		t.Errorf("POST /user with a duplicate email: status %d, want 409, body %s", duplicate.status, duplicate.raw)
	}

	// 4. Partial update. The request carries *string fields, so a nil email
	// must leave the column alone; sending only username is the check.
	updated := callGeneratedAPI(t, http.MethodPut, baseURL+"/user/"+strconv.Itoa(id), map[string]any{
		"username": "alice-updated",
	})
	if updated.status != http.StatusOK {
		t.Fatalf("PUT /user/%d: status %d, body %s", id, updated.status, updated.raw)
	}
	if got := jsonString(t, updated.body, "data", "username"); got != "alice-updated" {
		t.Errorf("PUT /user/%d: data.username = %q, want alice-updated", id, got)
	}
	if got := jsonString(t, updated.body, "data", "email"); got != "alice@example.com" {
		t.Errorf("PUT /user/%d: data.email = %q, want the untouched alice@example.com", id, got)
	}

	// 5. Pagination. Three rows, page size two: the first window is full, the
	// second holds the remainder, and total counts every row. The generated
	// repository computes offset and limit, so an off-by-one lands here.
	for _, name := range []string{"bob", "carol"} {
		resp := callGeneratedAPI(t, http.MethodPost, baseURL+"/user", map[string]any{
			"username": name,
			"email":    name + "@example.com",
		})
		if resp.status != http.StatusOK {
			t.Fatalf("POST /user (%s): status %d, body %s", name, resp.status, resp.raw)
		}
	}

	firstPage := callGeneratedAPI(t, http.MethodGet, baseURL+"/users?page=1&page_size=2", nil)
	if firstPage.status != http.StatusOK {
		t.Fatalf("GET /users page 1: status %d, body %s", firstPage.status, firstPage.raw)
	}
	if got := jsonInt(t, firstPage.body, "total"); got != 3 {
		t.Errorf("GET /users page 1: total = %d, want 3", got)
	}
	if got := len(jsonList(t, firstPage.body, "data")); got != 2 {
		t.Errorf("GET /users page 1: %d rows, want 2", got)
	}

	secondPage := callGeneratedAPI(t, http.MethodGet, baseURL+"/users?page=2&page_size=2", nil)
	if secondPage.status != http.StatusOK {
		t.Fatalf("GET /users page 2: status %d, body %s", secondPage.status, secondPage.raw)
	}
	// Ordering defaults to the primary key, so the window that starts at offset
	// two holds exactly the row created last. An order key that silently fell
	// back to nothing would still return a row here; the name is what pins it.
	rows := jsonList(t, secondPage.body, "data")
	if len(rows) != 1 {
		t.Fatalf("GET /users page 2: %d rows, want 1", len(rows))
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("GET /users page 2: row is %T, want an object", rows[0])
	}
	if got, _ := row["username"].(string); got != "carol" {
		t.Errorf("GET /users page 2: first row username = %q, want carol", got)
	}

	// 6. Delete, then read and delete again. A delete is the only write whose
	// success the wire cannot show by echoing the record, so its effect is
	// asserted through two follow-up requests: 404 on read, 404 on delete.
	//
	// The body is sent because this is the generated contract: every non-GET
	// call decodes its request object from JSON, and the generated SDK posts
	// that object on DELETE too. Sending nothing would fail the decode before
	// any of this — see the note on the interop gap in the finding this
	// milestone records.
	deleted := callGeneratedAPI(t, http.MethodDelete, baseURL+"/user/"+strconv.Itoa(id), map[string]any{})
	if deleted.status != http.StatusOK {
		t.Fatalf("DELETE /user/%d: status %d, body %s", id, deleted.status, deleted.raw)
	}
	if success, _ := deleted.body["success"].(bool); !success {
		t.Errorf("DELETE /user/%d: success = %v, want true", id, deleted.body["success"])
	}

	if gone := callGeneratedAPI(t, http.MethodGet, baseURL+"/user/"+strconv.Itoa(id), nil); gone.status != http.StatusNotFound {
		t.Errorf("GET /user/%d after delete: status %d, want 404", id, gone.status)
	}
	if again := callGeneratedAPI(t, http.MethodDelete, baseURL+"/user/"+strconv.Itoa(id), map[string]any{}); again.status != http.StatusNotFound {
		t.Errorf("DELETE /user/%d twice: status %d, want 404", id, again.status)
	}

	// The first delete must not have taken the other rows with it: a delete
	// built as an unscoped statement would still answer 200.
	remaining := callGeneratedAPI(t, http.MethodGet, baseURL+"/users?page=1&page_size=10", nil)
	if got := jsonInt(t, remaining.body, "total"); got != 2 {
		t.Errorf("GET /users after delete: total = %d, want 2", got)
	}

	// 7. The declarative surface, over the same table and the same process.
	// -resource makes microgen declare each table as a resource.Collection and
	// mount its REST routes; this is the only gate that runs those routes
	// against a driver, and the only one that checks both bindings see one table.
	exerciseGeneratedResourceSurface(t, baseURL)
}

// waitForGeneratedService waits for the readiness probe with a deadline that
// survives a saturated suite, and fails fast when the process is gone. The
// shared waitServer helper gives a process two seconds, which is right for an
// example server and wrong here: this one opens a database and migrates a table
// before it listens, while a dozen other packages are compiling in parallel.
func waitForGeneratedService(t *testing.T, url string, exited <-chan error, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			t.Fatalf("generated service exited before serving %s: %v\n%s", url, err, output.String())
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("generated service did not serve %s within %s\n%s", url, 60*time.Second, output.String())
}

// requireCToolchain reports the missing capability rather than letting the
// build fail with a cgo error the reader has to decode.
func requireCToolchain(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "0" {
			t.Skip("CGO_ENABLED=0: the generated main imports gorm.io/driver/sqlite")
		}
	}
	for _, candidate := range []string{"gcc", "cc", "clang"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return
		}
	}
	t.Skip("no C compiler on PATH: the generated sqlite driver needs cgo")
}

// pinGeneratedModule points the generated project at this checkout. A
// generated go.mod names the last published version, and resolving it would
// mean reading a module the test just changed.
func pinGeneratedModule(t *testing.T, dir, root string) {
	t.Helper()
	goMod := filepath.Join(dir, "go.mod")
	source := readFile(t, goMod)
	source += "\nreplace github.com/dreamsxin/go-kit/v2 => " + strconv.Quote(filepath.ToSlash(root)) + "\n"
	if err := os.WriteFile(goMod, []byte(source), 0o644); err != nil {
		t.Fatalf("pin generated module: %v", err)
	}
}

// installGeneratedCRUDBodies replaces the five stubs with the shortest correct
// implementation over the generated repository.
//
// Every replacement is anchored on the exact generated text and fails when the
// anchor is not found exactly once, so a template change surfaces as "this
// fixture no longer patches the generator" rather than as a compile error in a
// temporary directory.
func installGeneratedCRUDBodies(t *testing.T, serviceFile, importPath string) {
	t.Helper()
	source := readFile(t, serviceFile)

	source = replaceGeneratedOnce(t, source,
		"\tidl \""+importPath+"\"\n",
		"\tidl \""+importPath+"\"\n\n"+
			"\t\""+importPath+"/model\"\n"+
			"\t\""+importPath+"/repository\"\n\n"+
			"\t\"github.com/dreamsxin/go-kit/v2/apperror\"\n",
		"the service import block")

	stubs := []struct {
		name     string
		request  string
		response string
		body     string
	}{
		{
			name:     "CreateUser",
			request:  "CreateUserRequest",
			response: "CreateUserResponse",
			body: `	record := &model.User{Username: req.Username, Email: req.Email}
	if err := s.repos.UserRepo.Create(ctx, record); err != nil {
		if errors.Is(err, repository.ErrDuplicated) {
			return idl.CreateUserResponse{}, apperror.Wrap(apperror.KindAlreadyExists, "user.email_exists", "a user with that email already exists", err)
		}
		return idl.CreateUserResponse{}, err
	}
	return idl.CreateUserResponse{Data: userItem(record)}, nil
`,
		},
		{
			name:     "GetUser",
			request:  "GetUserRequest",
			response: "GetUserResponse",
			body: `	record, err := s.repos.UserRepo.GetByID(ctx, req.ID)
	if err != nil {
		return idl.GetUserResponse{}, err
	}
	if record == nil {
		return idl.GetUserResponse{}, apperror.New(apperror.KindNotFound, "user.not_found", "user not found")
	}
	return idl.GetUserResponse{Data: userItem(record)}, nil
`,
		},
		{
			name:     "UpdateUser",
			request:  "UpdateUserRequest",
			response: "UpdateUserResponse",
			body: `	values := map[string]interface{}{}
	if req.Username != nil {
		values["username"] = *req.Username
	}
	if req.Email != nil {
		values["email"] = *req.Email
	}
	if len(values) > 0 {
		err := s.repos.UserRepo.Updates(ctx, req.ID, values)
		if errors.Is(err, repository.ErrNotFound) {
			return idl.UpdateUserResponse{}, apperror.Wrap(apperror.KindNotFound, "user.not_found", "user not found", err)
		}
		if errors.Is(err, repository.ErrDuplicated) {
			return idl.UpdateUserResponse{}, apperror.Wrap(apperror.KindAlreadyExists, "user.email_exists", "a user with that email already exists", err)
		}
		if err != nil {
			return idl.UpdateUserResponse{}, err
		}
	}
	record, err := s.repos.UserRepo.GetByID(ctx, req.ID)
	if err != nil {
		return idl.UpdateUserResponse{}, err
	}
	if record == nil {
		return idl.UpdateUserResponse{}, apperror.New(apperror.KindNotFound, "user.not_found", "user not found")
	}
	return idl.UpdateUserResponse{Data: userItem(record)}, nil
`,
		},
		{
			name:     "DeleteUser",
			request:  "DeleteUserRequest",
			response: "DeleteUserResponse",
			body: `	err := s.repos.UserRepo.Delete(ctx, req.ID)
	if errors.Is(err, repository.ErrNotFound) {
		return idl.DeleteUserResponse{}, apperror.Wrap(apperror.KindNotFound, "user.not_found", "user not found", err)
	}
	if err != nil {
		return idl.DeleteUserResponse{}, err
	}
	return idl.DeleteUserResponse{Success: true}, nil
`,
		},
		{
			name:     "ListUsers",
			request:  "ListUsersRequest",
			response: "ListUsersResponse",
			body: `	result, err := s.repos.UserRepo.List(ctx, repository.PageQuery{
		Page:     req.Page,
		PageSize: req.PageSize,
		Keyword:  req.Keyword,
	})
	if err != nil {
		return idl.ListUsersResponse{}, err
	}
	items := make([]idl.UserItem, 0, len(result.Data))
	for i := range result.Data {
		items = append(items, *userItem(&result.Data[i]))
	}
	return idl.ListUsersResponse{Data: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize}, nil
`,
		},
	}

	for _, stub := range stubs {
		old := "func (s *serviceImpl) " + stub.name + "(ctx context.Context, req idl." + stub.request + ") (idl." + stub.response + ", error) {\n" +
			"\t_ = req\n" +
			"\treturn idl." + stub.response + "{}, errors.New(\"" + stub.name + ": not implemented\")\n" +
			"}"
		source = replaceGeneratedOnce(t, source, old,
			"func (s *serviceImpl) "+stub.name+"(ctx context.Context, req idl."+stub.request+") (idl."+stub.response+", error) {\n"+stub.body+"}",
			stub.name+" stub")
	}

	source += `
// userItem maps the stored model onto the generated DTO.
func userItem(record *model.User) *idl.UserItem {
	return &idl.UserItem{ID: record.ID, Username: record.Username, Email: record.Email}
}
`
	if err := os.WriteFile(serviceFile, []byte(source), 0o644); err != nil {
		t.Fatalf("write generated service: %v", err)
	}
}

func replaceGeneratedOnce(t *testing.T, source, old, replacement, what string) string {
	t.Helper()
	switch strings.Count(source, old) {
	case 1:
		return strings.Replace(source, old, replacement, 1)
	case 0:
		t.Fatalf("generated project: %s missing; the microgen template changed and this fixture no longer patches it", what)
	default:
		t.Fatalf("generated project: %s appears %d times, want 1", what, strings.Count(source, old))
	}
	return source
}

// generatedAPIResponse keeps the raw body for failure messages: a status alone
// does not say which layer disagreed.
type generatedAPIResponse struct {
	status int
	body   map[string]any
	raw    string
	header http.Header
}

func callGeneratedAPI(t *testing.T, method, url string, payload any) generatedAPIResponse {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode %s %s: %v", method, url, err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, url, err)
	}
	body := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %v\n%s", method, url, err, raw)
		}
	}
	return generatedAPIResponse{status: response.StatusCode, body: body, raw: string(raw), header: response.Header}
}

// jsonField walks nested objects. A missing key is reported as an empty value
// rather than a panic, so the assertion that follows names the field.
func jsonField(body map[string]any, keys ...string) any {
	current := any(body)
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[key]
	}
	return current
}

func jsonString(t *testing.T, body map[string]any, keys ...string) string {
	t.Helper()
	value, ok := jsonField(body, keys...).(string)
	if !ok {
		t.Fatalf("field %s is missing or not a string: %v", strings.Join(keys, "."), body)
	}
	return value
}

func jsonInt(t *testing.T, body map[string]any, keys ...string) int {
	t.Helper()
	value, ok := jsonField(body, keys...).(float64)
	if !ok {
		t.Fatalf("field %s is missing or not a number: %v", strings.Join(keys, "."), body)
	}
	return int(value)
}

func jsonList(t *testing.T, body map[string]any, keys ...string) []any {
	t.Helper()
	value, ok := jsonField(body, keys...).([]any)
	if !ok {
		t.Fatalf("field %s is missing or not a list: %v", strings.Join(keys, "."), body)
	}
	return value
}

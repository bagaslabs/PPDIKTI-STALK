package ppdikti

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	return &Client{
		httpClient:     upstream.Client(),
		searchEndpoint: upstream.URL + "/api/pencarian/enc/all/",
		detailEndpoint: upstream.URL + "/api/detail/mhs/",
	}
}

func TestSearchStudentList(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "001") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"mahasiswa":[{"id":"student-1","nama":"Nama Contoh","nim":"261910201001","nama_pt":"UNIVERSITAS JEMBER","nama_prodi":"Teknik"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"mahasiswa":[]}}`))
	}))
	client.requestInterval = 0

	students, err := client.SearchStudentList(context.Background(), Config{
		Universitas: "UNIVERSITAS JEMBER",
		Angkatan:    "26",
		AbsenMulai:  1,
		Format:      "1910201",
	})
	if err != nil {
		t.Fatalf("SearchStudentList() error = %v", err)
	}
	if len(students) != 1 || students[0].Nama != "Nama Contoh" {
		t.Fatalf("SearchStudentList() = %+v, want one matching student", students)
	}
}

func TestSearchStudentListWithProgress(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"mahasiswa":[]}}`))
	}))
	client.requestInterval = 0

	var searchedNIMs []string
	_, err := client.SearchStudentListWithProgress(context.Background(), Config{
		Universitas: "UNIVERSITAS JEMBER",
		Angkatan:    "26",
		AbsenMulai:  1,
		Format:      "1910201",
	}, func(nim string) {
		searchedNIMs = append(searchedNIMs, nim)
	})
	if err != nil {
		t.Fatalf("SearchStudentListWithProgress() error = %v", err)
	}
	if len(searchedNIMs) != 10 || searchedNIMs[0] != "261910201001" || searchedNIMs[9] != "261910201010" {
		t.Fatalf("searched NIMs = %v, want NIMs 261910201001 through 261910201010", searchedNIMs)
	}
}

func TestSearchStudentListWithCallbacksReportsFoundStudent(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "001") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"mahasiswa":[{"id":"student-1","nama":"Nama Contoh","nim":"261910201001","nama_pt":"UNIVERSITAS JEMBER","nama_prodi":"Teknik"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"mahasiswa":[]}}`))
	}))
	client.requestInterval = 0

	var found []Student
	students, err := client.SearchStudentListWithCallbacks(context.Background(), Config{
		Universitas: "UNIVERSITAS JEMBER",
		Angkatan:    "26",
		AbsenMulai:  1,
		Format:      "1910201",
	}, nil, func(student Student) error {
		found = append(found, student)
		return nil
	})
	if err != nil {
		t.Fatalf("SearchStudentListWithCallbacks() error = %v", err)
	}
	if len(found) != 1 || len(students) != 1 || found[0].ID != students[0].ID {
		t.Fatalf("found = %+v, students = %+v; want same single student", found, students)
	}
}

func TestGetStudentDetail(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/detail/mhs/student-1" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/detail/mhs/student-1")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"nama":"Nama Contoh","jenis_kelamin":"Laki-laki","nim":"261910201001","status":"Aktif"}}`))
	}))

	detail, err := client.GetStudentDetail(context.Background(), "student-1")
	if err != nil {
		t.Fatalf("GetStudentDetail() error = %v", err)
	}
	var response struct {
		Data struct {
			Nama         string `json:"nama"`
			JenisKelamin string `json:"jenis_kelamin"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail, &response); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if response.Data.Nama != "Nama Contoh" || response.Data.JenisKelamin != "Laki-laki" || response.Data.Status != "Aktif" {
		t.Fatalf("unexpected detail: %+v", response.Data)
	}
}

func TestStudentFunctionsReportUpstream403(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	client.requestInterval = time.Second
	config := Config{Universitas: "UNIVERSITAS JEMBER", Angkatan: "26", AbsenMulai: 1, Format: "1910201"}

	if _, err := client.SearchStudentList(context.Background(), config); err == nil || !strings.Contains(err.Error(), "HTTP 403 Forbidden") {
		t.Fatalf("SearchStudentList() error = %v, want upstream 403", err)
	}
	if _, err := client.GetStudentDetail(context.Background(), "student-1"); err == nil || !strings.Contains(err.Error(), "HTTP 403 Forbidden") {
		t.Fatalf("GetStudentDetail() error = %v, want upstream 403", err)
	}
}

func TestSearchStudentListRejectsInvalidConfig(t *testing.T) {
	client := NewClient()
	_, err := client.SearchStudentList(context.Background(), Config{})
	if err == nil {
		t.Fatal("SearchStudentList() with empty config must fail")
	}
}

func TestNewClientWithHTTPClientRejectsNil(t *testing.T) {
	if _, err := NewClientWithHTTPClient(nil); err == nil {
		t.Fatal("NewClientWithHTTPClient(nil) must fail")
	}
}

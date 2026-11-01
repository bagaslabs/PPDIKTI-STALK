package ppdikti

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	pddiktiSearchURL        = "https://pddikti.kemdiktisaintek.go.id/api/pencarian/enc/all/"
	pddiktiStudentDetailURL = "https://pddikti.kemdiktisaintek.go.id/api/detail/mhs/"
	pddiktiRequestInterval  = time.Second
	missingNIMLimit         = 10
	maxResponseSize         = 5 << 20
)

// Client calls the public PDDikti endpoints used by this package.
type Client struct {
	httpClient      *http.Client
	searchEndpoint  string
	detailEndpoint  string
	requestInterval time.Duration
}

// NewClient creates a PDDikti client with the default HTTP timeout.
func NewClient() *Client {
	return &Client{
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		searchEndpoint:  pddiktiSearchURL,
		detailEndpoint:  pddiktiStudentDetailURL,
		requestInterval: pddiktiRequestInterval,
	}
}

// NewClientWithHTTPClient creates a PDDikti client using the provided HTTP client.
func NewClientWithHTTPClient(httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("HTTP client tidak boleh nil")
	}
	client := NewClient()
	client.httpClient = httpClient
	return client, nil
}

// Config contains the values used to construct consecutive student NIMs.
type Config struct {
	Universitas string `json:"universitas"`
	Angkatan    string `json:"angkatan"`
	AbsenMulai  int    `json:"absen_mulai"`
	Format      string `json:"format"`
}

// Student contains the student fields returned by PDDikti search.
type Student struct {
	ID        string `json:"id"`
	Nama      string `json:"nama"`
	NIM       string `json:"nim"`
	NamaPT    string `json:"nama_pt"`
	NamaProdi string `json:"nama_prodi"`
}

type searchResponse struct {
	Status string `json:"status"`
	Data   struct {
		Mahasiswa []Student `json:"mahasiswa"`
	} `json:"data"`
}

// LoadConfig reads and validates a search configuration from a JSON file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("membaca konfigurasi: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("memproses konfigurasi: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks whether the configuration can be used to search for students.
func (config Config) Validate() error {
	if strings.TrimSpace(config.Universitas) == "" {
		return errors.New("universitas belum diisi di config.json")
	}
	if strings.TrimSpace(config.Angkatan) == "" {
		return errors.New("angkatan belum diisi di config.json")
	}
	if strings.TrimSpace(config.Format) == "" {
		return errors.New("format belum diisi di config.json")
	}
	if config.AbsenMulai < 1 || config.AbsenMulai > 999 {
		return errors.New("absen_mulai harus berada di antara 1 dan 999")
	}
	return nil
}

func (config Config) nim(absen int) (string, error) {
	if absen < 1 || absen > 999 {
		return "", errors.New("nomor absen harus berada di antara 1 dan 999")
	}
	return fmt.Sprintf("%s%s%03d", config.Angkatan, config.Format, absen), nil
}

// SearchStudentList looks up students using the NIM pattern in config.
// It stops after 10 consecutive missing NIMs or attendance number 999.
func (client *Client) SearchStudentList(ctx context.Context, config Config) ([]Student, error) {
	return client.SearchStudentListWithProgress(ctx, config, nil)
}

// SearchStudentListWithProgress looks up students and calls onSearch before each NIM request.
func (client *Client) SearchStudentListWithProgress(ctx context.Context, config Config, onSearch func(nim string)) ([]Student, error) {
	return client.SearchStudentListWithCallbacks(ctx, config, onSearch, nil)
}

// SearchStudentListWithCallbacks reports each NIM before searching and each student as soon as found.
func (client *Client) SearchStudentListWithCallbacks(ctx context.Context, config Config, onSearch func(nim string), onStudent func(Student) error) ([]Student, error) {
	if err := client.validate(); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	students, _, err := client.searchStudentList(ctx, config, onSearch, onStudent)
	return students, err
}

// GetStudentDetail returns the upstream JSON detail for a student ID.
func (client *Client) GetStudentDetail(ctx context.Context, id string) (json.RawMessage, error) {
	if err := client.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("ID mahasiswa tidak boleh kosong")
	}
	return getStudentDetails(ctx, client.httpClient, client.detailEndpoint, id)
}

func (client *Client) validate() error {
	if client == nil || client.httpClient == nil {
		return errors.New("PDDikti client belum diinisialisasi")
	}
	return nil
}

func searchStudents(ctx context.Context, client *http.Client, endpoint string, config Config, nim string) ([]Student, error) {
	searchTerm := strings.TrimSpace(config.Universitas) + " " + nim
	escapedSearchTerm := url.PathEscape(searchTerm)
	requestURL := strings.TrimRight(endpoint, "/") + "/" + escapedSearchTerm
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("membuat permintaan PDDikti: %w", err)
	}
	apiBase, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("memproses alamat API PDDikti: %w", err)
	}
	origin := apiBase.Scheme + "://" + apiBase.Host
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/search/"+escapedSearchTerm)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mengakses PDDikti: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("PDDikti mengembalikan status HTTP %s", resp.Status)
	}

	var result searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result); err != nil {
		return nil, fmt.Errorf("membaca respons PDDikti: %w", err)
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("PDDikti mengembalikan status %q", result.Status)
	}

	matches := make([]Student, 0, len(result.Data.Mahasiswa))
	for _, student := range result.Data.Mahasiswa {
		if strings.TrimSpace(student.NIM) == nim &&
			strings.EqualFold(strings.TrimSpace(student.NamaPT), strings.TrimSpace(config.Universitas)) {
			matches = append(matches, student)
		}
	}
	return matches, nil
}

func (client *Client) searchStudentList(ctx context.Context, config Config, onSearch func(nim string), onStudent func(Student) error) ([]Student, string, error) {
	students := make([]Student, 0)
	missingCount := 0
	for attendance := config.AbsenMulai; attendance <= 999; attendance++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		nim, err := config.nim(attendance)
		if err != nil {
			return nil, "", err
		}
		if onSearch != nil {
			onSearch(nim)
		}
		matches, err := searchStudents(ctx, client.httpClient, client.searchEndpoint, config, nim)
		if err != nil {
			return nil, "", fmt.Errorf("mencari NIM %s: %w", nim, err)
		}
		if len(matches) == 0 {
			missingCount++
			if missingCount >= missingNIMLimit {
				return students, nim, nil
			}
		} else {
			missingCount = 0
			students = append(students, matches...)
			if onStudent != nil {
				for _, student := range matches {
					if err := onStudent(student); err != nil {
						return nil, "", fmt.Errorf("memproses mahasiswa NIM %s: %w", student.NIM, err)
					}
				}
			}
		}

		if client.requestInterval > 0 && attendance < 999 {
			timer := time.NewTimer(client.requestInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, "", ctx.Err()
			case <-timer.C:
			}
		}
	}
	return students, "", nil
}

func getStudentDetails(ctx context.Context, client *http.Client, endpoint, id string) (json.RawMessage, error) {
	escapedID := url.PathEscape(id)
	requestURL := strings.TrimRight(endpoint, "/") + "/" + escapedID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("membuat permintaan detail PDDikti: %w", err)
	}
	apiBase, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("memproses alamat API PDDikti: %w", err)
	}
	origin := apiBase.Scheme + "://" + apiBase.Host
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mengakses detail mahasiswa PDDikti: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("PDDikti mengembalikan status HTTP %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("membaca detail mahasiswa PDDikti: %w", err)
	}
	if !json.Valid(data) {
		return nil, errors.New("PDDikti mengembalikan JSON detail mahasiswa yang tidak valid")
	}
	return json.RawMessage(data), nil
}

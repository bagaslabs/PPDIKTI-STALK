package main

import (
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

const pddiktiSearchURL = "https://pddikti.kemdiktisaintek.go.id/api/pencarian/enc/all/"
const pddiktiRequestInterval = time.Second
const missingNIMLimit = 10

type Config struct {
	Universitas string `json:"universitas"`
	Angkatan    string `json:"angkatan"`
	AbsenMulai  int    `json:"absen_mulai"`
	Format      string `json:"format"`
}

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

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("membaca konfigurasi: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("memproses konfigurasi: %w", err)
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (config Config) validate() error {
	if config.Universitas == "" {
		return errors.New("universitas belum diisi di config.json")
	}
	if config.Angkatan == "" {
		return errors.New("angkatan belum diisi di config.json")
	}
	if config.Format == "" {
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

func searchStudents(client *http.Client, endpoint string, config Config, nim string) ([]Student, error) {
	searchTerm := strings.TrimSpace(config.Universitas) + " " + nim
	escapedSearchTerm := url.PathEscape(searchTerm)
	requestURL := strings.TrimRight(endpoint, "/") + "/" + escapedSearchTerm
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&result); err != nil {
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

func reportProgress(progress io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(progress, format, args...); err != nil {
		return fmt.Errorf("menampilkan progress pencarian: %w", err)
	}
	return nil
}

func searchConsecutiveStudents(client *http.Client, endpoint string, config Config, interval time.Duration, progress io.Writer) ([]Student, string, error) {
	students := make([]Student, 0)
	missingCount := 0
	for attendance := config.AbsenMulai; attendance <= 999; attendance++ {
		nim, err := config.nim(attendance)
		if err != nil {
			return nil, "", err
		}

		if err := reportProgress(progress, "Mencari NIM %s...\n", nim); err != nil {
			return nil, "", err
		}
		matches, err := searchStudents(client, endpoint, config, nim)
		if err != nil {
			return nil, "", fmt.Errorf("mencari NIM %s: %w", nim, err)
		}
		if len(matches) == 0 {
			missingCount++
			if err := reportProgress(progress, "NIM %s tidak ditemukan (%d/%d berturut-turut).\n", nim, missingCount, missingNIMLimit); err != nil {
				return nil, "", err
			}
			if missingCount >= missingNIMLimit {
				return students, nim, nil
			}
		} else {
			missingCount = 0
			students = append(students, matches...)
			if err := reportProgress(progress, "NIM %s ditemukan.\n%s", nim, formatResults(matches, "", "")); err != nil {
				return nil, "", err
			}
		}

		if interval > 0 && attendance < 999 {
			time.Sleep(interval)
		}
	}
	return students, "", nil
}

func formatResults(students []Student, nim, universitas string) string {
	if len(students) == 0 {
		return fmt.Sprintf("Tidak ditemukan mahasiswa dengan NIM %s di %s.\n", nim, universitas)
	}

	var output strings.Builder
	for _, student := range students {
		fmt.Fprintf(&output, "Nama: %s\nNIM: %s\nUniversitas: %s\nProgram Studi: %s\nID: %s\n\n",
			student.Nama, student.NIM, student.NamaPT, student.NamaProdi, student.ID)
	}
	return output.String()
}

func writeResults(path string, students []Student, nim, universitas string) error {
	output := formatResults(students, nim, universitas)
	if err := os.WriteFile(path, []byte(output), 0644); err != nil {
		return fmt.Errorf("menyimpan hasil ke %s: %w", path, err)
	}
	return nil
}

func main() {
	config, err := loadConfig("config.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	students, missingNIM, err := searchConsecutiveStudents(client, pddiktiSearchURL, config, pddiktiRequestInterval, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeResults("list.txt", students, missingNIM, config.Universitas); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Ditemukan %d data mahasiswa.\n", len(students))
	if missingNIM != "" {
		fmt.Printf("Pencarian dihentikan setelah %d NIM kosong berturut-turut, terakhir %s.\n", missingNIMLimit, missingNIM)
	} else {
		fmt.Println("Seluruh rentang nomor absen sampai 999 sudah diperiksa.")
	}
	if len(students) == 0 {
		fmt.Print(formatResults(students, missingNIM, config.Universitas))
	}
}

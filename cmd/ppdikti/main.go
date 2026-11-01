package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	ppdikti "ppdikti_stalk_go"
)

func main() {
	config, err := ppdikti.LoadConfig("config.json")
	if err != nil {
		exitWithError(err)
	}

	client := ppdikti.NewClient()
	var output strings.Builder
	students, err := client.SearchStudentListWithCallbacks(context.Background(), config, func(nim string) {
		fmt.Printf("Mencari NIM %s...\n", nim)
	}, func(student ppdikti.Student) error {
		fmt.Printf("Ditemukan: %s (%s). Mengambil detail...\n", student.Nama, student.NIM)
		detail, err := client.GetStudentDetail(context.Background(), student.ID)
		if err != nil {
			return fmt.Errorf("mengambil detail: %w", err)
		}
		formattedDetail, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			return fmt.Errorf("memformat detail: %w", err)
		}
		record := fmt.Sprintf("Nama: %s\nNIM: %s\nUniversitas: %s\nProgram Studi: %s\nID: %s\nDetail:\n%s\n\n",
			student.Nama, student.NIM, student.NamaPT, student.NamaProdi, student.ID, formattedDetail)
		if _, err := io.WriteString(os.Stdout, record); err != nil {
			return fmt.Errorf("menampilkan hasil: %w", err)
		}
		if _, err := output.WriteString(record); err != nil {
			return fmt.Errorf("menyimpan hasil sementara: %w", err)
		}
		return nil
	})
	if err != nil {
		exitWithError(err)
	}

	if err := os.WriteFile("list.txt", []byte(output.String()), 0644); err != nil {
		exitWithError(fmt.Errorf("menyimpan hasil ke list.txt: %w", err))
	}
	fmt.Printf("Ditemukan %d data mahasiswa.\n", len(students))
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

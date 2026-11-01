# PPDIKTI Stalk Go

Library Go untuk mencari list mahasiswa PDDikti berdasarkan pola NIM dan mengambil detail mahasiswa berdasarkan ID. Tidak ada HTTP server lokal; fungsi dipanggil langsung dari program Go.

## API library dan nilai return

Import package dari kode Go:

```go
import ppdikti "ppdikti_stalk_go"
```

API ini terdiri dari `Client` untuk request ke PDDikti, `Config` untuk menyusun pola NIM, dan `Student` untuk menampung hasil pencarian. `Client` harus dibuat dengan salah satu constructor; field internalnya tidak diekspos.

Client mengakses endpoint publik PDDikti berikut:

- Pencarian: `https://pddikti.kemdiktisaintek.go.id/api/pencarian/enc/all/`
- Detail mahasiswa: `https://pddikti.kemdiktisaintek.go.id/api/detail/mhs/`

Akses bergantung pada ketersediaan dan kebijakan upstream; HTTP 403 dikembalikan sebagai error dan tidak dicoba untuk dilewati.

### `NewClient() *Client`

Membuat client PDDikti dengan pengaturan bawaan:

- HTTP timeout 15 detik per request.
- Jeda 1 detik di antara request saat mencari list.
- Menggunakan endpoint pencarian dan detail PDDikti yang dikonfigurasi di package.

**Return:** pointer ke `Client` yang siap digunakan. Constructor ini tidak menghubungi PDDikti, sehingga belum bisa memastikan endpoint dapat diakses.

### `NewClientWithHTTPClient(httpClient *http.Client) (*Client, error)`

Membuat client dengan `http.Client` pilihan pemanggil. Berguna untuk memasang transport, proxy yang memang diizinkan, atau timeout sendiri. Jeda pencarian tetap 1 detik.

**Return:**

- `*Client`: client yang siap digunakan.
- `error`: `nil` jika berhasil; error jika `httpClient` bernilai `nil`.

Contoh:

```go
import (
	"log"
	"net/http"
	"time"
)

httpClient := &http.Client{Timeout: 30 * time.Second}
client, err := ppdikti.NewClientWithHTTPClient(httpClient)
if err != nil {
	log.Fatal(err)
}
```

### `LoadConfig(path string) (Config, error)`

Membaca file JSON dan memvalidasi konfigurasi sebelum mengembalikannya.

**Parameter:** `path` adalah path file konfigurasi.

**Return:**

- `Config`: konfigurasi hasil pembacaan jika berhasil; `Config{}` jika terjadi error.
- `error`: `nil` jika berhasil; selain itu error baca file, JSON tidak valid, atau nilai konfigurasi tidak memenuhi validasi `Config.Validate`.

### `Config`

```go
type Config struct {
	Universitas string `json:"universitas"`
	Angkatan    string `json:"angkatan"`
	AbsenMulai  int    `json:"absen_mulai"`
	Format      string `json:"format"`
}
```

- `Universitas`: nama perguruan tinggi untuk filter hasil.
- `Angkatan`: awalan angkatan pada NIM.
- `AbsenMulai`: nomor absen awal, dari 1 sampai 999.
- `Format`: bagian NIM di antara angkatan dan nomor absen.
- Field JSON yang diharapkan: `universitas`, `angkatan`, `absen_mulai`, dan `format`.

NIM dibentuk dari `Angkatan + Format + nomor absen`; nomor absen selalu tiga digit. Contohnya `26` + `1910201` + `1` menjadi `261910201001`.

### `(Config).Validate() error`

Memeriksa bahwa `Universitas`, `Angkatan`, dan `Format` tidak kosong setelah spasi di awal/akhir diabaikan, serta `AbsenMulai` berada pada rentang 1–999. Method ini tidak memvalidasi pola atau karakter numerik `Angkatan` dan `Format`.

**Return:** `nil` jika valid; selain itu error yang menjelaskan nilai yang tidak valid. Method ini juga dipanggil oleh `LoadConfig` dan method pencarian.

### `(*Client).SearchStudentList(ctx context.Context, config Config) ([]Student, error)`

Mencari mahasiswa berurutan berdasarkan pola NIM dalam konfigurasi. Request dijalankan satu per satu. Hanya hasil yang NIM-nya cocok persis dan perguruan tingginya cocok (tanpa membedakan kapitalisasi) yang dimasukkan.

Pencarian mulai dari `AbsenMulai` dan berhenti setelah 10 NIM berturut-turut tidak ditemukan atau setelah memeriksa nomor absen 999. Karena itu pencarian bisa menghasilkan list kosong jika tidak ada hasil yang cocok.

**Parameter:**

- `ctx`: context untuk membatalkan operasi atau memberikan deadline. Berikan `context.Background()` jika tidak memerlukan pembatalan.
- `config`: pola pencarian; harus lolos `config.Validate()`.

**Return:**

- `[]Student`: list mahasiswa yang cocok. Jika tidak ada mahasiswa, hasilnya slice kosong yang valid.
- `error`: `nil` jika berhasil; selain itu error validasi/client, pembatalan context atau deadline, kegagalan jaringan, status HTTP upstream non-2xx (termasuk 403), respons JSON pencarian yang tidak valid, atau status API upstream selain `success`. Jika error terjadi, slice hasil bernilai `nil`.

Hasil hanya diterima jika NIM sama persis dan nama perguruan tinggi sama tanpa membedakan kapitalisasi. Pencarian memeriksa setiap nomor absen secara berurutan; setiap hasil ditemukan akan mereset penghitung NIM yang hilang. Request diberi jeda bawaan satu detik. `ctx` diperiksa sebelum request dan selama jeda.

### `(*Client).SearchStudentListWithProgress(ctx context.Context, config Config, onSearch func(nim string)) ([]Student, error)`

Sama seperti `SearchStudentList`, tetapi memanggil `onSearch` tepat sebelum setiap request pencarian dikirim. Gunakan callback ini untuk menampilkan progres; callback boleh `nil`.

**Parameter:** `onSearch` menerima NIM yang akan diminta. Callback berjalan sinkron; fungsi ini menunggu callback selesai sebelum mengirim request.

**Return:** sama dengan `SearchStudentList`: slice mahasiswa dan error. Error callback tidak berlaku karena callback ini tidak memiliki return value.

### `(*Client).SearchStudentListWithCallbacks(ctx context.Context, config Config, onSearch func(nim string), onStudent func(Student) error) ([]Student, error)`

Memanggil `onSearch` sebelum setiap request dan `onStudent` segera setelah mahasiswa yang cocok dari request itu diterima, sebelum pencarian NIM berikutnya. Kedua callback boleh `nil`. Callback berjalan sinkron, sehingga dapat digunakan untuk menampilkan progres, mengambil detail, atau memproses hasil secara langsung.

**Parameter callback:**

- `onSearch`: menerima NIM yang akan dicari; tidak memiliki return value.
- `onStudent`: menerima satu `Student` yang lolos filter. Mengembalikan `nil` untuk melanjutkan atau error untuk menghentikan pencarian.

**Return:** sama dengan `SearchStudentList`. Jika `onStudent` mengembalikan error, pencarian berhenti, hasil yang dikembalikan `nil`, dan error callback dibungkus dengan informasi NIM. Callback tidak dipanggil untuk NIM tanpa hasil yang cocok.

### `Student`

Setiap hasil pencarian bertipe:

```go
type Student struct {
	ID        string `json:"id"`
	Nama      string `json:"nama"`
	NIM       string `json:"nim"`
	NamaPT    string `json:"nama_pt"`
	NamaProdi string `json:"nama_prodi"`
}
```

- `ID`: ID yang bisa dipakai untuk meminta detail mahasiswa.
- `Nama`: nama mahasiswa.
- `NIM`: nomor induk mahasiswa.
- `NamaPT`: nama perguruan tinggi.
- `NamaProdi`: nama program studi.

Struct ini merupakan data ringkas dari endpoint pencarian. Data detail tambahan didapat dari `GetStudentDetail`.

### `(*Client).GetStudentDetail(ctx context.Context, id string) (json.RawMessage, error)`

Mengambil detail mahasiswa dari endpoint detail PDDikti menggunakan ID. Respons diteruskan sebagai JSON mentah (`json.RawMessage`), bukan struct tetap, karena field detail mengikuti respons upstream dan dapat berubah.

**Parameter:**

- `ctx`: context untuk membatalkan request atau memberikan deadline.
- `id`: ID mahasiswa, biasanya `Student.ID` dari hasil `SearchStudentList`; tidak boleh kosong.

**Return:**

- `json.RawMessage`: byte JSON valid dari respons detail. Bentuknya tidak dipetakan ke struct package ini; bisa dicetak dengan `fmt.Println(string(detail))` atau di-decode oleh pemanggil menggunakan `json.Unmarshal`.
- `error`: `nil` jika berhasil; selain itu error jika client belum diinisialisasi, ID kosong, context dibatalkan/deadline, terjadi kegagalan jaringan, upstream memberi status HTTP non-2xx (termasuk 403), body gagal dibaca, atau respons bukan JSON valid. Jika error terjadi, hasil bernilai `nil`.

Method ini memvalidasi bahwa body adalah JSON yang valid, tetapi tidak mengharuskan field `status` bernilai `success`. Ukuran respons dibatasi hingga 5 MiB.

Field seperti nama, jenis kelamin, perguruan tinggi, program studi, dan status akademik hanya tersedia jika memang dikirim oleh upstream. Contoh membaca field dari respons JSON:

```go
var detail struct {
	Data map[string]any `json:"data"`
}
if err := json.Unmarshal(rawDetail, &detail); err != nil {
	log.Fatal(err)
}
fmt.Println(detail.Data)
```

### Contoh pemakaian lengkap

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	ppdikti "ppdikti_stalk_go"
)

func main() {
	config, err := ppdikti.LoadConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	client := ppdikti.NewClient()
	ctx := context.Background()
	students, err := client.SearchStudentList(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	for _, student := range students {
		fmt.Printf("%s - %s (%s)\n", student.Nama, student.NIM, student.ID)
	}

	if len(students) == 0 {
		return
	}
	rawDetail, err := client.GetStudentDetail(ctx, students[0].ID)
	if err != nil {
		log.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal(rawDetail, &detail); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", detail)
}
```

Contoh `config.json`:

```json
{
  "universitas": "UNIVERSITAS JEMBER",
  "angkatan": "26",
  "absen_mulai": 1,
  "format": "1910201"
}
```

NIM dibentuk dari `angkatan + format + absen`, dengan nomor absen selalu tiga digit. Contoh konfigurasi tersebut menghasilkan NIM pertama `261910201001`.

```json
Nama: BAGAS WICAKSONO
NIM: 261910201072
Universitas: UNIVERSITAS JEMBER
Program Studi: TEKNIK ELEKTRO
ID: ytqVUm0oI22la2byWqR3BQrCktJT05nvx2eWHiMQJ6tWN8L3S-hC0N-TQZYzh8sNU4hqOQ==
Detail:
{
  "status": "success",
  "data": {
    "id": "MxxkFVh3aNawRS0LvMSnaIz5di_lBvqewk7pIDkONoUgP3KQ1G53gKz5E-kP639s2cbSWw==",
    "nama_pt": "Universitas Jember",
    "kode_pt": "001025  ",
    "kode_prodi": "20201",
    "prodi": "Teknik Elektro",
    "nama": "BAGAS WICAKSONO",
    "nim": "261910201072",
    "jenis_daftar": "Peserta didik baru",
    "id_pt": "sGEpjYh3ihx48SvzDSBdmAHMZd0R-fYOLeCADKvWyjrOtowvSVa46GT3W1aLI12b6b4iEw==",
    "id_sms": "_leqT5r_0Sx7eSu8fZkc245FTlvU63PmMhp8vhrXUOuk8oMknEtUi1lB4Po_Jw0VDJlB6w==",
    "jenis_kelamin": "L",
    "jenjang": "Sarjana",
    "status_saat_ini": "Aktif-2026/2027 Ganjil",
    "tanggal_masuk": "2026-08-18"
  }
}
```


## Menjalankan CLI

CLI disediakan sebagai contoh penggunaan library. Dari folder proyek:

```sh
go run ./cmd/ppdikti
```

CLI membaca `config.json` dari working directory saat ini, mencetak NIM yang sedang dicari, lalu langsung menampilkan data dan detail JSON setiap mahasiswa saat ditemukan. Setelah pencarian selesai, hasil ditulis ke `list.txt` (file lama ditimpa). Jika proses dihentikan sebelum pencarian selesai, file hasil belum ditulis.

## Batasan akses

Library memanggil endpoint publik PDDikti secara langsung. Jika upstream mengembalikan HTTP 403, error tersebut dikembalikan ke pemanggil; library tidak mencoba melewati proteksi upstream. Detail mahasiswa hanya berisi field yang dikirim PDDikti.

## Tes

```sh
go test ./...
```

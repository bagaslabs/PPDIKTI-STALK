# PPDIKTI Stalk

Aplikasi Go untuk mencari data mahasiswa melalui PDDikti berdasarkan universitas dan NIM.

## Konfigurasi

Atur nilai di `config.json`:

```json
{
  "universitas": "UNIVERSITAS JEMBER",
  "angkatan": "26",
  "absen_mulai": 1,
  "format": "1910201"
}
```

NIM dibentuk dari `angkatan + format + absen`, dengan nomor absen selalu ditulis dalam tiga digit. Contoh konfigurasi di atas menghasilkan NIM pertama `261910201001`.

## Menjalankan

Pastikan Go sudah terpasang, lalu jalankan dari folder proyek:

```sh
go run .
```

Aplikasi mencari NIM satu per satu mulai dari `absen_mulai`. Setiap GET ditunggu sampai selesai, dengan jeda satu detik sebelum request berikutnya. Progress pencarian dan data yang ditemukan ditampilkan langsung di console.

Nomor absen dapat memiliki jeda. Karena itu pencarian baru berhenti setelah 10 NIM berturut-turut tidak ditemukan, atau setelah memeriksa absen 999. Hasil yang cocok disimpan dengan menimpa `list.txt` di folder proyek. File tersebut tidak dilacak Git karena berisi data hasil pencarian.

Untuk memeriksa kompilasi:

```sh
go test ./...
```

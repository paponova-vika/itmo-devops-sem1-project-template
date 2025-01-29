package main

import (
	"archive/zip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	dbHost     = "localhost"
	dbPort     = 5432
	dbUser     = "validator"
	dbPassword = "val1dat0r"
	dbName     = "project-sem-1"
)

var db *sql.DB

func main() {
	var err error
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPassword, dbName)
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}

	if err := initDatabase(); err != nil {
		panic(fmt.Sprintf("Failed to initialize database: %v", err))
	}

	http.HandleFunc("/api/v0/prices", handlePrices)
	fmt.Println("Server is running on port 8080")
	http.ListenAndServe(":8080", nil)
}

func initDatabase() error {
	// Проверяем подключение
	if err := db.Ping(); err != nil {
		return err
	}

	// Создаем таблицу, если её нет
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS prices (
        id SERIAL PRIMARY KEY,
        product_name TEXT NOT NULL,
        category TEXT NOT NULL,
        price NUMERIC(10,2) NOT NULL,
        creation_date DATE NOT NULL
    )`)
	return err
}

func handlePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handlePostPrices(w, r)
	} else if r.Method == http.MethodGet {
		handleGetPrices(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePostPrices(w http.ResponseWriter, r *http.Request) {
	log.Println("Received POST request to /api/v0/prices")

	// Читаем zip-файл из запроса
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Println("Error: Failed to read uploaded file:", err)
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Создаем временную директорию
	tempDir, err := os.MkdirTemp("", "uploaded")
	if err != nil {
		log.Println("Error: Failed to create temporary directory:", err)
		http.Error(w, "Failed to create temp directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	// Сохраняем загруженный архив
	zipPath := filepath.Join(tempDir, "uploaded.zip")
	outFile, err := os.Create(zipPath)
	if err != nil {
		log.Println("Error: Failed to create temp zip file:", err)
		http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, file)
	if err != nil {
		log.Println("Error: Failed to save zip file:", err)
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	log.Println("Successfully saved uploaded zip file:", zipPath)

	// Открываем ZIP-архив
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Println("Error: Failed to open zip file:", err)
		http.Error(w, "Failed to open zip file", http.StatusInternalServerError)
		return
	}
	defer zipReader.Close()

	var csvFilePath string

	// Ищем CSV-файл внутри архива
	for _, zipFile := range zipReader.File {
		if zipFile.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(zipFile.Name), ".csv") {
			continue
		}

		log.Println("Found CSV file inside ZIP:", zipFile.Name)

		// Сохраняем CSV в временную папку
		fileName := filepath.Base(zipFile.Name)
		csvFilePath = filepath.Join(tempDir, fileName)
		outFile, err := os.Create(csvFilePath)
		if err != nil {
			log.Println("Error: Failed to create CSV file:", err)
			http.Error(w, "Failed to create CSV file", http.StatusInternalServerError)
			return
		}
		defer outFile.Close()

		zipFileReader, err := zipFile.Open()
		if err != nil {
			log.Println("Error: Failed to read CSV file from archive:", err)
			http.Error(w, "Failed to read file from archive", http.StatusInternalServerError)
			return
		}
		defer zipFileReader.Close()

		_, err = io.Copy(outFile, zipFileReader)
		if err != nil {
			log.Println("Error: Failed to extract CSV file:", err)
			http.Error(w, "Failed to extract CSV file", http.StatusInternalServerError)
			return
		}

		log.Println("Successfully extracted CSV file:", csvFilePath)
		break
	}

	if csvFilePath == "" {
		log.Println("Error: No CSV file found in archive")
		http.Error(w, "No CSV file found in archive", http.StatusBadRequest)
		return
	}

	// Открываем CSV-файл
	csvFile, err := os.Open(csvFilePath)
	if err != nil {
		log.Println("Error: Failed to open extracted CSV file:", err)
		http.Error(w, "data.csv not found in archive", http.StatusBadRequest)
		return
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	reader.Comma = ','

	rows, err := reader.ReadAll()
	if err != nil {
		log.Println("Error: Failed to parse CSV file:", err)
		http.Error(w, "Failed to parse CSV file", http.StatusInternalServerError)
		return
	}

	log.Printf("CSV file contains %d rows (including header)", len(rows))

	totalItems, totalCategories, totalPrice := 0, make(map[string]struct{}), 0.0
	for i, row := range rows {
		// Пропускаем заголовок
		if i == 0 {
			continue
		}

		if len(row) != 5 {
			log.Printf("Skipping malformed row %d: %v", i, row)
			continue
		}

		productID, err := strconv.Atoi(row[0])
		if err != nil {
			log.Printf("Skipping row %d due to invalid product ID: %v", i, row[0])
			continue
		}
		productName := row[1]
		category := row[2]
		price, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			log.Printf("Skipping row %d due to invalid price: %v", i, row[3])
			continue
		}
		creationDate := row[4]

		_, err = db.Exec(`INSERT INTO prices (id, product_name, category, price, creation_date) VALUES ($1, $2, $3, $4, $5)`,
			productID, productName, category, price, creationDate)
		if err != nil {
			log.Printf("Skipping row %d due to database error: %v", i, err)
			continue
		}

		totalItems++
		totalCategories[category] = struct{}{}
		totalPrice += price
	}

	log.Printf("Successfully inserted %d items into database", totalItems)

	response := map[string]interface{}{
		"total_items":      totalItems,
		"total_categories": len(totalCategories),
		"total_price":      totalPrice,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	log.Println("Response sent successfully")
}

func handleGetPrices(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, product_name, category, price, creation_date FROM prices`)
	if err != nil {
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tempDir, err := ioutil.TempDir("", "output")
	if err != nil {
		http.Error(w, "Failed to create temp directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	csvPath := filepath.Join(tempDir, "data.csv")
	csvFile, err := os.Create(csvPath)
	if err != nil {
		http.Error(w, "Failed to create CSV file", http.StatusInternalServerError)
		return
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)

	// Записываем заголовок
	err = writer.Write([]string{"id", "name", "category", "price", "create_date"})
	if err != nil {
		http.Error(w, "Failed write CSV header", http.StatusInternalServerError)
		return
	}

	// Записываем заголовок
	for rows.Next() {
		var productID int
		var creationDate, productName, category string
		var price float64
		err = rows.Scan(&productID, &productName, &category, &price, &creationDate)
		if err != nil {
			http.Error(w, "Failed to scan row", http.StatusInternalServerError)
			return
		}
		err = writer.Write([]string{
			strconv.Itoa(productID), productName, category, fmt.Sprintf("%.2f", price), creationDate,
		})
		if err != nil {
			http.Error(w, "Failed to write to CSV file", http.StatusInternalServerError)
			return
		}
	}

	writer.Flush()

	if err := writer.Error(); err != nil {
		http.Error(w, "Error during CSV write", http.StatusInternalServerError)
		return
	}

	zipPath := filepath.Join(tempDir, "data.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		http.Error(w, "Failed to create ZIP file", http.StatusInternalServerError)
		return
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	csvInArchive, err := archive.Create("data.csv")
	if err != nil {
		http.Error(w, "Failed to add CSV to ZIP", http.StatusInternalServerError)
		return
	}

	_, err = csvFile.Seek(0, io.SeekStart)
	if err != nil {
		http.Error(w, "Failed to seek CSV file", http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(csvInArchive, csvFile)
	if err != nil {
		http.Error(w, "Failed to copy CSV to ZIP", http.StatusInternalServerError)
		return
	}

	err = archive.Close()

	if err != nil {
		http.Error(w, "Failed to close ZIP archive", http.StatusInternalServerError)
		return
	}

	_, err = zipFile.Seek(0, io.SeekStart)
	if err != nil {
		http.Error(w, "Failed to seek ZIP file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=data.zip")

	_, err = io.Copy(w, zipFile)
	if err != nil {
		http.Error(w, "Failed to send ZIP file", http.StatusInternalServerError)
		return
	}
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		filePath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		fileInArchive, err := f.Open()
		if err != nil {
			destFile.Close()
			return err
		}
		_, err = io.Copy(destFile, fileInArchive)
		destFile.Close()
		fileInArchive.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

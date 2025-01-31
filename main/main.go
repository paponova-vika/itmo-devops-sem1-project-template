package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"io"
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

	defer db.Close()

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
        creation_date timestamp NOT NULL
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

	// Читаем содержимое ZIP в память
	zipData, err := io.ReadAll(file)
	if err != nil {
		log.Println("Error: Failed to read ZIP file into memory:", err)
		http.Error(w, "Failed to read ZIP file", http.StatusInternalServerError)
		return
	}

	// Открываем ZIP-архив из памяти
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		log.Println("Error: Failed to open ZIP file:", err)
		http.Error(w, "Failed to open ZIP file", http.StatusInternalServerError)
		return
	}

	var csvFile io.ReadCloser

	// Ищем CSV-файл внутри архива
	for _, zipFile := range zipReader.File {
		if zipFile.FileInfo().IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(zipFile.Name), ".csv") {
			continue
		}

		log.Println("Found CSV file inside ZIP:", zipFile.Name)

		csvFile, err := zipFile.Open()
		if err != nil {
			log.Println("Error: Failed to open CSV file:", err)
			http.Error(w, "Failed to open CSV file", http.StatusInternalServerError)
			return
		}

		log.Println("Successfully extracted CSV file:", csvFile)
		defer csvFile.Close()
		break
	}

	if csvFile == nil {
		log.Println("Error: No CSV file found in archive")
		http.Error(w, "No CSV file found in archive", http.StatusBadRequest)
		return
	}

	reader := csv.NewReader(csvFile)
	reader.Comma = ','

	rows, err := reader.ReadAll()
	if err != nil {
		log.Println("Error: Failed to parse CSV file:", err)
		http.Error(w, "Failed to parse CSV file", http.StatusInternalServerError)
		return
	}

	log.Printf("CSV file contains %d rows (including header)", len(rows))

	// Проверяем данные перед вставкой в БД
	products := []struct {
		ProductID    int
		ProductName  string
		Category     string
		Price        float64
		CreationDate string
	}{}

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

		price, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			log.Printf("Skipping row %d due to invalid price: %v", i, row[3])
			continue
		}

		products = append(products, struct {
			ProductID    int
			ProductName  string
			Category     string
			Price        float64
			CreationDate string
		}{
			ProductID:    productID,
			ProductName:  row[1],
			Category:     row[2],
			Price:        price,
			CreationDate: row[4],
		})
	}

	if len(products) == 0 {
		log.Println("Error: No valid data found in CSV")
		http.Error(w, "No valid data in CSV", http.StatusBadRequest)
		return
	}

	// Начинаем транзакцию для массовой вставки
	tx, err := db.Begin()
	if err != nil {
		log.Println("Error: Failed to start transaction:", err)
		http.Error(w, "Database transaction error", http.StatusInternalServerError)
		return
	}

	stmt, err := tx.Prepare(`INSERT INTO prices (product_name, category, price, creation_date) VALUES ($2, $3, $4, $5)`)
	if err != nil {
		log.Println("Error: Failed to prepare statement:", err)
		tx.Rollback()
		http.Error(w, "Database preparation error", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	totalItems := 0
	for _, p := range products {
		_, err = stmt.Exec(p.ProductID, p.ProductName, p.Category, p.Price, p.CreationDate)
		if err != nil {
			log.Printf("Skipping row with Product ID %d due to database error: %v", p.ProductID, err)
			continue
		}
		totalItems++
		//totalCategories[p.Category] = struct{}{}
		//totalPrice += p.Price
	}

	err = tx.Commit()
	if err != nil {
		log.Println("Error: Failed to commit transaction:", err)
		http.Error(w, "Database commit error", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully inserted %d items into database", totalItems)

	// Запрашиваем статистику из БД
	var totalCategories int
	var totalPrice float64
	err = db.QueryRow(`SELECT COUNT(DISTINCT category), SUM(price) FROM prices`).Scan(&totalCategories, &totalPrice)
	if err != nil {
		log.Println("Error: Failed to calculate statistics:", err)
		http.Error(w, "Failed to calculate statistics", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"total_items":      totalItems,
		"total_categories": totalCategories,
		"total_price":      totalPrice,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	log.Println("Response sent successfully")
}

func handleGetPrices(w http.ResponseWriter, r *http.Request) {
	// Запрашиваем все данные из БД
	rows, err := db.Query(`SELECT id, product_name, category, price, creation_date FROM prices`)
	if err != nil {
		http.Error(w, "Failed to query database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Сохраняем данные в память
	type Product struct {
		ID           int
		Name         string
		Category     string
		Price        float64
		CreationDate string
	}
	var products []Product

	for rows.Next() {
		var p Product
		err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.CreationDate)
		if err != nil {
			log.Println("Error: Failed to scan row:", err)
			http.Error(w, "Failed to scan row", http.StatusInternalServerError)
			return
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		log.Println("Error: Error occurred while iterating over rows:", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	log.Printf("Loaded %d products from database", len(products))

	// Создаем CSV в памяти
	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)

	// Записываем заголовок
	err = writer.Write([]string{"id", "name", "category", "price", "create_date"})
	if err != nil {
		log.Println("Error: Failed to write CSV header:", err)
		http.Error(w, "Failed to write CSV header", http.StatusInternalServerError)
		return
	}

	// Записываем строки
	for _, p := range products {
		err = writer.Write([]string{
			strconv.Itoa(p.ID),
			p.Name,
			p.Category,
			fmt.Sprintf("%.2f", p.Price),
			p.CreationDate,
		})
		if err != nil {
			log.Println("Error: Failed to write CSV row:", err)
			http.Error(w, "Failed to write CSV row", http.StatusInternalServerError)
			return
		}
	}
	writer.Flush()

	if err := writer.Error(); err != nil {
		log.Println("Error: CSV writing error:", err)
		http.Error(w, "CSV writing error", http.StatusInternalServerError)
		return
	}

	log.Println("CSV file created in memory")

	// Создаем ZIP-архив в памяти
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)

	csvFile, err := zipWriter.Create("data.csv")
	if err != nil {
		log.Println("Error: Failed to create CSV inside ZIP:", err)
		http.Error(w, "Failed to create CSV inside ZIP", http.StatusInternalServerError)
		return
	}

	_, err = csvFile.Write(csvBuffer.Bytes())
	if err != nil {
		log.Println("Error: Failed to write CSV to ZIP:", err)
		http.Error(w, "Failed to write CSV to ZIP", http.StatusInternalServerError)
		return
	}

	err = zipWriter.Close()
	if err != nil {
		log.Println("Error: Failed to close ZIP archive:", err)
		http.Error(w, "Failed to close ZIP archive", http.StatusInternalServerError)
		return
	}

	log.Println("ZIP archive created in memory")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
	w.Header().Set("Content-Length", strconv.Itoa(zipBuffer.Len()))
	_, err = w.Write(zipBuffer.Bytes())

	if err != nil {
		log.Println("Error: Failed to send ZIP file:", err)
		http.Error(w, "Failed to send ZIP file", http.StatusInternalServerError)
		return
	}

	log.Println("ZIP file sent successfully")
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

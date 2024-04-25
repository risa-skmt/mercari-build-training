package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	_ "modernc.org/sqlite"
)

const (
	ImgDir = "images"
	dbPath = "../mercari.sqlite3"
)

type Response struct {
	Message string `json:"message"`
}

type Items struct {
	Items []Item `json:"items"`
}

type Item struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func addItem(c echo.Context) error {
	// Get form data
	name := c.FormValue("name")
	category := c.FormValue("category")
	// get form file
	file, err := c.FormFile("image")
	if err != nil {
		log.Print("画像ファイルの受け取りに失敗", err)
		return err
	}

	c.Logger().Infof("Receive item: %s", name)
	c.Logger().Infof("Receive item: %s", category)
	c.Logger().Infof("Receive item: %s", file)

	message := fmt.Sprintf("item received: %s", name)
	res := Response{Message: message}

	// open image file
	src, err := file.Open()
	if err != nil {
		log.Print("画像ファイルの読み取りに失敗", err)
		return err
	}
	defer src.Close()

	// create hash
	h := sha256.New()
	if _, err = io.Copy(h, src); err != nil {
		log.Print("hash生成に失敗", err)
		return err
	}
	imageName := fmt.Sprintf("%x.jpg", h.Sum(nil))

	// select directory path for new image file
	filePath := filepath.Join("images/", imageName)
	// destination to store image file
	dst, err := os.Create(filePath)
	if err != nil {
		log.Print("新規画像ファイル作成に失敗", err)
		return err
	}
	defer dst.Close()

	// copy image to created image file
	if _, err = io.Copy(dst, src); err != nil {
		log.Print("新規画像の保存に失敗", err)
		return err
	}
	// connect to db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Print("db接続に失敗")
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer db.Close()

	// categoryテーブルに存在してなかった時の書きたい
	// result, err := db.Query("SELECT EXISTS(SELECT * FROM category WHERE name = ?)", category)
	// log.Print(category, result)
    // if err != nil{
	// 	_, err = db.Exec("INSERT INTO category name VALUES ?", category)
	// 	if err != nil{
	// 		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	// 	}
	// }

	stmt, err := db.Prepare("INSERT INTO items (name, category_id, image_name) VALUES (?,(SELECT id FROM category WHERE name = ?),?)")
	if err != nil {
		log.Print("INSERTクエリ失敗")
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer stmt.Close()
	_, err = stmt.Exec(name, category, imageName)
	if err != nil {
		log.Print("INSERT失敗")
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, res)
}

func getItems(c echo.Context) error {
	// connect to db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Print("db接続に失敗")
		return err
	}
	defer db.Close()
	// get data from db
	rows, err := db.Query("SELECT items.id, items.name, category.name, items.image_name FROM ITEMS INNER JOIN category on items.category_id = category.id")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()
	items := Items{}

	for rows.Next() {
		var item Item
		err := rows.Scan(&item.Id, &item.Name, &item.Category, &item.Image)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items.Items = append(items.Items, item)
	}
	return c.JSON(http.StatusOK, items)
}

func getItemById(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	// connect to db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Print("db接続に失敗")
		return err
	}
	defer db.Close()
	// get data from db
	rows, err := db.Query("SELECT ITEMS.name, category.name, ITEMS.image_name FROM ITEMS INNER JOIN category on ITEMS.category_id = category.id")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := Items{}

	for rows.Next() {
		var item Item
		err := rows.Scan(&item.Name, &item.Category, &item.Image)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items.Items = append(items.Items, item)
	}

	defer rows.Close()
	// check if id is safe number
	if id > len(items.Items) || id <= 0 {
		log.Print("指定されたidが不正な値です")
		return err
	}
	selectedItemById := items.Items[id-1]

	return c.JSON(http.StatusOK, selectedItemById)
}

func searchItem(c echo.Context) error {
	keyword := c.QueryParam("keyword")

	// connect to db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Print("db接続に失敗")
		return err
	}
	defer db.Close()

	rows, err := db.Query("SELECT items.id, items.name, category.name, items.image_name FROM items INNER JOIN category ON items.category_id = category.id WHERE items.name LIKE ?", "%"+keyword+"%")
	if err != nil {
		log.Print("クエリ失敗")
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	items := Items{}

	for rows.Next() {
		var item Item
		err := rows.Scan(&item.Id, &item.Name, &item.Category, &item.Image)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items.Items = append(items.Items, item)
	}
	return c.JSON(http.StatusOK, items)
}

func getImg(c echo.Context) error {
	// Create image path
	imgPath := path.Join(ImgDir, c.Param("imageFilename"))

	if !strings.HasSuffix(imgPath, ".jpg") {
		res := Response{Message: "Image path does not end with .jpg"}
		return c.JSON(http.StatusBadRequest, res)
	}
	if _, err := os.Stat(imgPath); err != nil {
		c.Logger().Debugf("Image not found: %s", imgPath)
		imgPath = path.Join(ImgDir, "default.jpg")
	}
	return c.File(imgPath)
}

func main() {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Logger.SetLevel(log.DEBUG)

	frontURL := os.Getenv("FRONT_URL")
	if frontURL == "" {
		frontURL = "http://localhost:3000"
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{frontURL},
		AllowMethods: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete},
	}))

	// Routes
	e.GET("/", root)
	e.POST("/items", addItem)
	e.GET("/items", getItems)
	e.GET("/items/:id", getItemById)
	e.GET("/image/:imageFilename", getImg)
	e.GET("/search", searchItem)

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}

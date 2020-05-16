// +build windows,amd64package main
package main

import (
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/kbinani/screenshot"
)

func main() {
	start := time.Now()
	bounds := getWindowInfoByName("Diablo III")

	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		panic(err)
	}
	fileName := fmt.Sprintf("%dx%d.png", bounds.Dx(), bounds.Dy())
	file, _ := os.Create(fileName)
	defer file.Close()
	png.Encode(file, img)
	fmt.Printf("%v \"%s\"\n", bounds, fileName)

	elapsed := time.Since(start)

	fmt.Printf("Elapsed: %s", elapsed)
}

//

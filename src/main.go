package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	"go/parser"
)

const (
	inputVideo     = "input.mp4"
	inputFrameDir  = "input_frames"
	shaderFrameDir = "shader_frames"
	blendedDir     = "blended_frames"
	outputVideo    = "output/output.mp4"
	frameRate      = 30
)

func main() {
	ensureDirs()

	fmt.Println("Extracting frames from input.mp4...")
	if err := runFFmpegExtract(); err != nil {
		log.Fatalf("ffmpeg extract error: %v", err)
	}

	fmt.Println("Detecting frame size...")
	width, height, err := getFrameSize()
	if err != nil {
		log.Fatalf("cannot detect size: %v", err)
	}

	files, err := os.ReadDir(inputFrameDir)
	if err != nil {
		log.Fatalf("read frame count: %v", err)
	}

	// Define shader expressions here — can be dynamic or loaded from CLI/config
	rExpr := "(x*y + frame) % 255"
	gExpr := "(y*y + frame) % 255"
	bExpr := "(x*x / (frame + 1)) % 255"
	aExpr := "255"

	fmt.Println("Generating shader frames...")
	if err := runShaderFrameGen(len(files), width, height, rExpr, gExpr, bExpr, aExpr); err != nil {
		log.Fatalf("shader frame gen error: %v", err)
	}

	fmt.Println("Blending frames...")
	if err := blendAllFrames(); err != nil {
		log.Fatalf("blend error: %v", err)
	}

	fmt.Println("Re-encoding output video...")
	if err := runFFmpegAssemble(outputVideo, filepath.Join(blendedDir, "blend_%04d.png"), frameRate); err != nil {
		log.Fatalf("ffmpeg assemble error: %v", err)
	}

	fmt.Println("Done!")
}

func runFFmpegExtract() error {
	cmd := exec.Command("ffmpeg", "-i", inputVideo, filepath.Join(inputFrameDir, "frame_%04d.png"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runFFmpegAssemble(output string, inputPattern string, fps int) error {
	cmd := exec.Command("ffmpeg", "-framerate", strconv.Itoa(fps), "-i", inputPattern, "-c:v", "libx264", "-pix_fmt", "yuv420p", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func blendImages(img1, img2 image.Image) image.Image {
	out := image.NewRGBA(img1.Bounds())
	draw.Draw(out, img1.Bounds(), img1, image.Point{}, draw.Over)
	draw.Draw(out, img2.Bounds(), img2, image.Point{}, draw.Over) // change to draw.Src for hard replace
	return out
}

func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

func saveImage(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func ensureDirs() {
	os.MkdirAll(inputFrameDir, 0755)
	os.MkdirAll(shaderFrameDir, 0755)
	os.MkdirAll(blendedDir, 0755)
	os.MkdirAll("output", 0755)
}

func runShaderFrameGen(frameCount int, width, height int, rExpr, gExpr, bExpr, aExpr string) error {
	for i := 0; i < frameCount; i++ {
		filename := fmt.Sprintf(filepath.Join(shaderFrameDir, "shader_%04d.png"), i+1)
		fmt.Println("Generating shader frame:", filename)
		err := simulateShaderRender(i, width, height, filename, rExpr, gExpr, bExpr, aExpr)
		if err != nil {
			return err
		}
	}
	return nil
}


func simulateShaderRender(frameIndex, width, height int, outPath string, rExprStr, gExprStr, bExprStr, aExprStr string) error {
	rExpr, err := parser.ParseExpr(rExprStr)
	if err != nil {
		return err
	}
	gExpr, err := parser.ParseExpr(gExprStr)
	if err != nil {
		return err
	}
	bExpr, err := parser.ParseExpr(bExprStr)
	if err != nil {
		return err
	}
	aExpr, err := parser.ParseExpr(aExprStr)
	if err != nil {
		return err
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			vars := map[string]int{
				"x":     x,
				"y":     y,
				"frame": frameIndex,
			}

			r, _ := evalExprTreeNode(rExpr, vars)
			g, _ := evalExprTreeNode(gExpr, vars)
			b, _ := evalExprTreeNode(bExpr, vars)
			a, _ := evalExprTreeNode(aExpr, vars)

			img.Set(x, y, color.RGBA{
				R: clampToByte(r),
				G: clampToByte(g),
				B: clampToByte(b),
				A: clampToByte(a),
			})
		}
	}

	return saveImage(outPath, img)
}


func blendAllFrames() error {
	files, err := os.ReadDir(inputFrameDir)
	if err != nil {
		return err
	}
	for i, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".png") {
			continue
		}
		framePath := filepath.Join(inputFrameDir, f.Name())
		shaderPath := filepath.Join(shaderFrameDir, fmt.Sprintf("shader_%04d.png", i+1))
		outputPath := filepath.Join(blendedDir, fmt.Sprintf("blend_%04d.png", i+1))

		src, err1 := loadImage(framePath)
		shd, err2 := loadImage(shaderPath)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("error loading: %v / %v", err1, err2)
		}
		blended := blendImages(src, shd)
		if err := saveImage(outputPath, blended); err != nil {
			return err
		}
	}
	return nil
}

func getFrameSize() (int, int, error) {
	files, err := os.ReadDir(inputFrameDir)
	if err != nil || len(files) == 0 {
		return 0, 0, fmt.Errorf("no frames found")
	}
	img, err := loadImage(filepath.Join(inputFrameDir, files[0].Name()))
	if err != nil {
		return 0, 0, err
	}
	b := img.Bounds()
	return b.Dx(), b.Dy(), nil
}

func clampToByte(val float64) uint8 {
	if val < 0 {
		return 0
	}
	if val > 255 {
		return 255
	}
	return uint8(val)
}
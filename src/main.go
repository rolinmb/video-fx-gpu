package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/go-gl/gl/v3.2-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	width      = 0 // Will be detected from video
	height     = 0
	frameRate  = 30
	frameCount = 0
)

var quadVertices = []float32{
	-1.0, -1.0,
	1.0, -1.0,
	-1.0, 1.0,
	1.0, 1.0,
}

func init() {
	runtime.LockOSThread()
}

func main() {
	// Step 1: Extract frames from input.mp4
	exec.Command("mkdir", "-p", "input_frames").Run()
	cmd := exec.Command("ffmpeg", "-y", "-i", "input/input.mp4", "input_frames/frame_%03d.png")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal("ffmpeg extract failed:", err)
	}

	// Detect width and height
	imgFile, err := os.Open("input_frames/frame_000.png")
	if err != nil {
		log.Fatal(err)
	}
	img, _, err := image.Decode(imgFile)
	if err != nil {
		log.Fatal(err)
	}
	width = img.Bounds().Dx()
	height = img.Bounds().Dy()
	imgFile.Close()

	files, _ := filepath.Glob("input_frames/frame_*.png")
	frameCount = len(files)

	// Step 2: Init GLFW and GL
	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to init glfw:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 2)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Visible, glfw.False)

	window, err := glfw.CreateWindow(width, height, "Offscreen", nil, nil)
	if err != nil {
		log.Fatalln("failed to create window:", err)
	}
	window.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		log.Fatalln("failed to init OpenGL:", err)
	}

	vertexShaderSrc := `#version 150 core
		in vec2 vert;
		out vec2 uv;
		void main() {
			uv = vert * 0.5 + 0.5;
			gl_Position = vec4(vert, 0.0, 1.0);
		}` + "\x00"

	fragmentShaderSrc := `#version 150 core
		in vec2 uv;
		out vec4 color;
		uniform float u_time;
		void main() {
			float r = 0.5 + 0.5 * sin(u_time + uv.x * 10.0);
			float g = 0.5 + 0.5 * cos(u_time + uv.y * 10.0);
			float b = r * g;
			color = vec4(r, g, b, 1.0);
		}` + "\x00"

	program := createProgram(vertexShaderSrc, fragmentShaderSrc)
	gl.UseProgram(program)
	uTimeLoc := gl.GetUniformLocation(program, gl.Str("u_time\x00"))

	var vao, vbo uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(quadVertices)*4, gl.Ptr(quadVertices), gl.STATIC_DRAW)
	vertAttrib := uint32(gl.GetAttribLocation(program, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(vertAttrib)
	gl.VertexAttribPointer(vertAttrib, 2, gl.FLOAT, false, 0, nil)

	var fbo, tex uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(width), int32(height), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0)

	_ = os.Mkdir("blended_frames", 0755)
	for frame := 0; frame < frameCount; frame++ {
		t := float32(frame) / float32(frameRate)
		gl.Uniform1f(uTimeLoc, t)
		gl.Viewport(0, 0, int32(width), int32(height))
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

		data := make([]uint8, width*height*4)
		gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(data))
		shaderImg := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := ((height-1-y)*width + x) * 4
				shaderImg.Set(x, y, color.RGBA{
					R: data[i], G: data[i+1], B: data[i+2], A: data[i+3],
				})
			}
		}

		inputFramePath := fmt.Sprintf("input_frames/frame_%03d.png", frame)
		f, _ := os.Open(inputFramePath)
		inputImg, _, _ := image.Decode(f)
		f.Close()

		blended := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(blended, blended.Bounds(), inputImg, image.Point{}, draw.Over)
		draw.DrawMask(blended, blended.Bounds(), shaderImg, image.Point{}, nil, image.Point{}, draw.Over)

		outPath := fmt.Sprintf("blended_frames/frame_%03d.png", frame)
		outFile, _ := os.Create(outPath)
		png.Encode(outFile, blended)
		outFile.Close()
		fmt.Println("Saved", outPath)
	}

	// Re-encode to output video
	_ = os.Mkdir("output", 0755)
	cmd = exec.Command("ffmpeg", "-y", "-framerate", strconv.Itoa(frameRate), "-i", "blended_frames/frame_%03d.png", "-c:v", "libx264", "-pix_fmt", "yuv420p", "output/output.mp4")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("Encoding final video...")
	if err := cmd.Run(); err != nil {
		log.Fatal("ffmpeg final encode failed:", err)
	}
	fmt.Println("✅ Final video written to output/output.mp4")
}

func compileShader(src string, shaderType uint32) uint32 {
	shader := gl.CreateShader(shaderType)
	csrc, free := gl.Strs(src)
	gl.ShaderSource(shader, 1, csrc, nil)
	free()
	gl.CompileShader(shader)
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		logMsg := make([]byte, logLength+1)
		gl.GetShaderInfoLog(shader, logLength, nil, &logMsg[0])
		log.Fatal("shader compile error: ", string(logMsg))
	}
	return shader
}

func createProgram(vertSrc, fragSrc string) uint32 {
	vs := compileShader(vertSrc, gl.VERTEX_SHADER)
	fs := compileShader(fragSrc, gl.FRAGMENT_SHADER)
	program := gl.CreateProgram()
	gl.AttachShader(program, vs)
	gl.AttachShader(program, fs)
	gl.LinkProgram(program)
	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		logMsg := make([]byte, logLength+1)
		gl.GetProgramInfoLog(program, logLength, nil, &logMsg[0])
		log.Fatal("link error: ", string(logMsg))
	}
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	return program
}

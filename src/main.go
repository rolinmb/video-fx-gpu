package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/go-gl/gl/v3.2-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	width     = 512
	height    = 512
	frameRate = 30
	numFrames = 60
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
	if err := glfw.Init(); err != nil {
		log.Fatalln("failed to init glfw:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 2)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Visible, glfw.False) // Offscreen

	window, err := glfw.CreateWindow(width, height, "Offscreen", nil, nil)
	if err != nil {
		log.Fatalln("failed to create window:", err)
	}
	window.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		log.Fatalln("failed to init OpenGL:", err)
	}
	fmt.Println("OpenGL version:", gl.GoStr(gl.GetString(gl.VERSION)))

	vertexShaderSrc := `
		#version 150 core
		in vec2 vert;
		out vec2 uv;
		void main() {
			uv = vert * 0.5 + 0.5;
			gl_Position = vec4(vert, 0.0, 1.0);
		}
	` + "\x00"

	fragmentShaderSrc := `
		#version 150 core
		in vec2 uv;
		out vec4 color;
		uniform float u_time;
		void main() {
			float r = 0.5 + 0.5 * sin(u_time + uv.x * 10.0);
			float g = 0.5 + 0.5 * cos(u_time + uv.y * 10.0);
			float b = r * g;
			color = vec4(r, g, b, 1.0);
		}
	` + "\x00"

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
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0)

	if gl.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
		log.Fatal("Framebuffer is not complete")
	}

	_ = os.Mkdir("frames", 0755)

	for frame := 0; frame < numFrames; frame++ {
		t := float32(frame) / float32(frameRate)
		gl.Uniform1f(uTimeLoc, t)

		gl.Viewport(0, 0, width, height)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

		data := make([]uint8, width*height*4)
		gl.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(data))

		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := ((height-1-y)*width + x) * 4
				img.Set(x, y, color.RGBA{
					R: data[i],
					G: data[i+1],
					B: data[i+2],
					A: data[i+3],
				})
			}
		}

		filename := fmt.Sprintf("frames/frame_%03d.png", frame)
		file, _ := os.Create(filename)
		defer file.Close()
		png.Encode(file, img)
		fmt.Println("Wrote", filename)
	}

	fmt.Println("✅ Animation frames complete")

	_ = os.Mkdir("output", 0755)
	cmd := exec.Command("ffmpeg",
		"-y", // overwrite
		"-framerate", fmt.Sprint(frameRate),
		"-i", "frames/frame_%03d.png",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"output/output.mp4",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("🎬 Running ffmpeg to produce output/output.mp4...")
	if err := cmd.Run(); err != nil {
		log.Fatal("ffmpeg failed:", err)
	}

	fmt.Println("✅ Video saved to output/output.mp4")
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
		log.Fatal("shader compile error:", string(logMsg))
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
		log.Fatal("link error:", string(logMsg))
	}

	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	return program
}

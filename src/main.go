package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

const (
	width       = 512
	height      = 512
	frameCount  = 60
	outputDir   = "shader_frames"
	vertexSrc   = `
#version 330 core
out vec2 uv;
void main() {
	float x = float((gl_VertexID << 1) & 2);
	float y = float(gl_VertexID & 2);
	uv = vec2(x, y);
	gl_Position = vec4(x * 2.0 - 1.0, y * 2.0 - 1.0, 0.0, 1.0);
}`
)

// Dynamic shader expression (replace these dynamically)
var redExpr = "0.5 + 0.5 * sin(time)"
var greenExpr = "0.5 + 0.5 * cos(time)"
var blueExpr = "0.5 + 0.5 * sin(time * 0.5)"

func init() {
	runtime.LockOSThread()
}

func main() {
	if err := glfw.Init(); err != nil {
		panic(err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Visible, glfw.False) // Offscreen rendering
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)

	window, err := glfw.CreateWindow(width, height, "ShaderRenderer", nil, nil)
	if err != nil {
		panic(err)
	}
	window.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		panic(err)
	}

	shaderSrc := fmt.Sprintf(`
#version 330 core
in vec2 uv;
out vec4 fragColor;
uniform float time;
void main() {
	float r = %s;
	float g = %s;
	float b = %s;
	fragColor = vec4(r, g, b, 1.0);
}
`, redExpr, greenExpr, blueExpr)

	prog := createProgram(vertexSrc, shaderSrc)
	gl.UseProgram(prog)

	timeLoc := gl.GetUniformLocation(prog, gl.Str("time\x00"))

	// Framebuffer + Texture
	var fbo, tex uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)

	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0)

	if gl.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
		panic("Framebuffer is not complete")
	}

	// Render & Save
	os.MkdirAll(outputDir, 0755)

	for i := 0; i < frameCount; i++ {
		t := float32(i) / float32(frameCount)
		gl.Viewport(0, 0, width, height)
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		gl.Uniform1f(timeLoc, t*6.28) // full cycle of sin/cos
		gl.DrawArrays(gl.TRIANGLES, 0, 3)

		img := readPixels(width, height)
		outFile := filepath.Join(outputDir, fmt.Sprintf("shader_%04d.png", i))
		f, err := os.Create(outFile)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
		fmt.Println("Wrote", outFile)
	}
}

func createProgram(vsSrc, fsSrc string) uint32 {
	vs := compileShader(vsSrc, gl.VERTEX_SHADER)
	fs := compileShader(fsSrc, gl.FRAGMENT_SHADER)
	prog := gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)

	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength+1)
		gl.GetProgramInfoLog(prog, logLength, nil, &log[0])
		panic(fmt.Sprintf("Shader link error: %s", log))
	}
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	return prog
}

func compileShader(src string, typ uint32) uint32 {
	shader := gl.CreateShader(typ)
	csources, free := gl.Strs(src + "\x00")
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength+1)
		gl.GetShaderInfoLog(shader, logLength, nil, &log[0])
		panic(fmt.Sprintf("Shader compile error: %s", log))
	}
	return shader
}

func readPixels(w, h int) image.Image {
	data := make([]uint8, w*h*4)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(data))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(img.Pix[y*w*4:(y+1)*w*4], data[(h-1-y)*w*4:(h-y)*w*4]) // Flip Y
	}
	return img
}

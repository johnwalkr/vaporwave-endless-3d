package vaporwave

import "strings"

const glesHeader = "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;\n"

// WebGLShaders returns the OpenGL renderer's shaders converted to GLSL ES
// 3.00 for WebGL 2 (the Quest web app), plus a blit that can draw into one
// eye's viewport of the headset's framebuffer.
func WebGLShaders() map[string]string {
	es := func(s string) string { return strings.Replace(s, "#version 330 core\n", glesHeader, 1) }
	return map[string]string{
		"fsVS":   es(fsVS),
		"bgFS":   es(bgFS()),
		"geoVS":  es(geoVS),
		"geoFS":  es(geoFS),
		"downFS": es(downFS),
		"blurFS": es(blurFS),
		"compFS": es(compFS),
		"blitFS": glesHeader + `uniform sampler2D src;
uniform vec4 P; // viewport x, y, w, h in the target framebuffer
out vec4 o;
void main(){
	vec2 uv=(gl_FragCoord.xy-P.xy)/P.zw;
	o=vec4(texture(src,vec2(uv.x,1.0-uv.y)).rgb,1.0);
}
`,
	}
}

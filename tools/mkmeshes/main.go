// mkmeshes wrote assets/meshes/*.glb once: the arrow, the sling stone and
// the sling's two poses, which the game used to build in code (voxellib's
// explorer arrow, a cube in the rock sprite's average colour, Nightfall's
// template_sling and template_sling_swinging block models). The glb files
// are the meshes now (weapons.rec names them); this is how they were made.
//
//	go run ./tools/mkmeshes
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
)

func main() {
	tex, err := os.Open("tools/mkmeshes/sling_model.png")
	if err != nil {
		panic(err)
	}
	slingTex, err := png.Decode(tex)
	if err != nil {
		panic(err)
	}
	for _, m := range []*model.Model{arrowModel(), stoneModel(), cuboidModel("sling_rest", slingRest, slingTex), cuboidModel("sling_swinging", slingSwinging, slingTex)} {
		path := filepath.Join("assets", "meshes", m.Name+".glb")
		if err := os.WriteFile(path, glb(m), 0o644); err != nil {
			panic(err)
		}
		back, err := gltf.LoadFile(path) // the check: what the game loads is what was built
		if err != nil {
			panic(err)
		}
		if fmt.Sprint(back.Meshes[0].Verts) != fmt.Sprint(m.Meshes[0].Verts) || fmt.Sprint(back.Meshes[0].Indices) != fmt.Sprint(m.Meshes[0].Indices) {
			panic(path + ": the file does not load back to the built mesh")
		}
		if fmt.Sprint(back.Textures[0].At(0, 0).RGBA()) != fmt.Sprint(m.Textures[0].At(0, 0).RGBA()) {
			panic(path + ": the texture does not load back")
		}
		fmt.Println(path, len(m.Meshes[0].Verts), "vertices")
	}
}

// glb packs a one-mesh model as binary glTF with its texture inside.
func glb(m *model.Model) []byte {
	me := m.Meshes[0]
	var bin bytes.Buffer
	var views []map[string]any
	view := func(data []byte) int {
		for bin.Len()%4 != 0 {
			bin.WriteByte(0)
		}
		views = append(views, map[string]any{"buffer": 0, "byteOffset": bin.Len(), "byteLength": len(data)})
		bin.Write(data)
		return len(views) - 1
	}
	floats := func(f []float32) []byte {
		b := make([]byte, 4*len(f))
		for i, v := range f {
			binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
		}
		return b
	}
	var pos, nor, uv []float32
	mn, mx := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, v := range me.Verts {
		pos, nor, uv = append(pos, v.Pos[:]...), append(nor, v.Normal[:]...), append(uv, v.UV[:]...)
		for k := range 3 {
			mn[k], mx[k] = min(mn[k], v.Pos[k]), max(mx[k], v.Pos[k])
		}
	}
	idx := make([]byte, 4*len(me.Indices))
	for i, v := range me.Indices {
		binary.LittleEndian.PutUint32(idx[i*4:], v)
	}
	var img bytes.Buffer
	if err := png.Encode(&img, m.Textures[0]); err != nil {
		panic(err)
	}
	accessors := []map[string]any{
		{"bufferView": view(floats(pos)), "componentType": 5126, "count": len(me.Verts), "type": "VEC3", "min": mn, "max": mx},
		{"bufferView": view(floats(nor)), "componentType": 5126, "count": len(me.Verts), "type": "VEC3"},
		{"bufferView": view(floats(uv)), "componentType": 5126, "count": len(me.Verts), "type": "VEC2"},
		{"bufferView": view(idx), "componentType": 5125, "count": len(me.Indices), "type": "SCALAR"},
	}
	imgView := view(img.Bytes())
	doc, err := json.Marshal(map[string]any{
		"asset": map[string]any{"version": "2.0"}, "scene": 0, "scenes": []any{map[string]any{"nodes": []int{0}}},
		"nodes":       []any{map[string]any{"name": m.Name, "mesh": 0}},
		"meshes":      []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]int{"POSITION": 0, "NORMAL": 1, "TEXCOORD_0": 2}, "indices": 3, "material": 0}}}},
		"materials":   []any{map[string]any{"pbrMetallicRoughness": map[string]any{"baseColorTexture": map[string]int{"index": 0}}}},
		"textures":    []any{map[string]any{"source": 0}},
		"images":      []any{map[string]any{"bufferView": imgView, "mimeType": "image/png"}},
		"buffers":     []any{map[string]any{"byteLength": bin.Len()}},
		"bufferViews": views, "accessors": accessors,
	})
	if err != nil {
		panic(err)
	}
	for len(doc)%4 != 0 {
		doc = append(doc, ' ')
	}
	for bin.Len()%4 != 0 {
		bin.WriteByte(0)
	}
	var out bytes.Buffer
	chunk := func(typ uint32, data []byte) {
		binary.Write(&out, binary.LittleEndian, uint32(len(data)))
		binary.Write(&out, binary.LittleEndian, typ)
		out.Write(data)
	}
	out.WriteString("glTF")
	binary.Write(&out, binary.LittleEndian, uint32(2))
	binary.Write(&out, binary.LittleEndian, uint32(12+8+len(doc)+8+bin.Len()))
	chunk(0x4E4F534A, doc)
	chunk(0x004E4942, bin.Bytes())
	return out.Bytes()
}

func bare(name string, tex image.Image) *model.Model {
	return &model.Model{Name: name, Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: name, Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
}

// arrowModel is voxellib's explorer arrow: pointing along +Z with its tip
// at the origin, a shaft, a head and fletching from a 4-pixel texture.
func arrowModel() *model.Model {
	tex := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	tex.Set(0, 0, color.NRGBA{120, 86, 52, 255})   // shaft
	tex.Set(1, 0, color.NRGBA{170, 170, 175, 255}) // head
	tex.Set(2, 0, color.NRGBA{235, 235, 235, 255}) // fletching
	tex.Set(3, 0, color.NRGBA{70, 70, 75, 255})    // nock
	m := bare("arrow", tex)
	var me model.Mesh
	box := func(lo, hi mgl32.Vec3, px int) { addBox(&me, lo, hi, (float32(px)+0.5)/4) }
	// head: a four-sided pyramid, its point at the origin
	u := (1 + 0.5) / float32(4)
	apex := mgl32.Vec3{0, 0, 0}
	tri := func(a, b, c mgl32.Vec3) {
		n := b.Sub(a).Cross(c.Sub(a)).Normalize()
		i := uint32(len(me.Verts))
		for _, p := range []mgl32.Vec3{a, b, c} {
			me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
		}
		me.Indices = append(me.Indices, i, i+1, i+2)
	}
	const s = 1.0 / 32 // half the shaft's thickness
	box(mgl32.Vec3{-s, -s, -0.75}, mgl32.Vec3{s, s, -0.28}, 0)
	c := [4]mgl32.Vec3{{-2 * s, -2 * s, -0.3}, {2 * s, -2 * s, -0.3}, {2 * s, 2 * s, -0.3}, {-2 * s, 2 * s, -0.3}}
	for k := 0; k < 4; k++ {
		tri(c[k], c[(k+1)%4], apex)
	}
	tri(c[0], c[2], c[1])
	tri(c[0], c[3], c[2])
	// fletching: two crossed vanes turned 45° about the shaft
	first := len(me.Verts)
	box(mgl32.Vec3{-0.002, -4 * s, -0.72}, mgl32.Vec3{0.002, 4 * s, -0.55}, 2)
	box(mgl32.Vec3{-4 * s, -0.002, -0.72}, mgl32.Vec3{4 * s, 0.002, -0.55}, 2)
	turn := mgl32.HomogRotate3DZ(math.Pi / 4)
	for i := first; i < len(me.Verts); i++ {
		v := &me.Verts[i]
		v.Pos = mgl32.TransformCoordinate(v.Pos, turn)
		v.Normal = mgl32.TransformNormal(v.Normal, turn)
	}
	box(mgl32.Vec3{-1.2 * s, -1.2 * s, -0.78}, mgl32.Vec3{1.2 * s, 1.2 * s, -0.74}, 3) // nock
	m.Meshes = []model.Mesh{me}
	return m
}

// stoneModel is a sling stone: a solid little cube of 0.25 blocks in the
// rock sprite's average colour.
func stoneModel() *model.Model {
	tex := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tex.Set(0, 0, color.NRGBA{104, 111, 140, 255})
	m := bare("stone", tex)
	var me model.Mesh
	const r = 0.25 / 2
	addBox(&me, mgl32.Vec3{-r, -r, -r}, mgl32.Vec3{r, r, r}, 0.5)
	m.Meshes = []model.Mesh{me}
	return m
}

// addBox appends a box's six faces to the mesh, all with the texture
// column u.
func addBox(me *model.Mesh, lo, hi mgl32.Vec3, u float32) {
	for _, f := range [6]struct {
		n mgl32.Vec3
		c [4]mgl32.Vec3
	}{
		{mgl32.Vec3{1, 0, 0}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {hi[0], hi[1], lo[2]}, {hi[0], hi[1], hi[2]}, {hi[0], lo[1], hi[2]}}},
		{mgl32.Vec3{-1, 0, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], hi[1], hi[2]}, {lo[0], hi[1], lo[2]}, {lo[0], lo[1], lo[2]}}},
		{mgl32.Vec3{0, 1, 0}, [4]mgl32.Vec3{{lo[0], hi[1], lo[2]}, {lo[0], hi[1], hi[2]}, {hi[0], hi[1], hi[2]}, {hi[0], hi[1], lo[2]}}},
		{mgl32.Vec3{0, -1, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], lo[1], lo[2]}, {hi[0], lo[1], lo[2]}, {hi[0], lo[1], hi[2]}}},
		{mgl32.Vec3{0, 0, 1}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {hi[0], lo[1], hi[2]}, {hi[0], hi[1], hi[2]}, {lo[0], hi[1], hi[2]}}},
		{mgl32.Vec3{0, 0, -1}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {lo[0], lo[1], lo[2]}, {lo[0], hi[1], lo[2]}, {hi[0], hi[1], lo[2]}}},
	} {
		base := uint32(len(me.Verts))
		for _, c := range f.c {
			me.Verts = append(me.Verts, model.Vertex{Pos: c, Normal: f.n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
		}
		me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
	}
}

// Nightfall's sling: block-model cuboids on 0..16 with per-face UV
// rectangles (x1, y1, x2, y2 on the 16px texture; the face order north,
// south, east, west, up, down).
type cuboid struct {
	from, to mgl32.Vec3
	uv       [6][4]float32
}

var (
	cordUV  = [6][4]float32{{0, 0, 1, 2}, {0, 0, 1, 2}, {0, 0, 1, 2}, {0, 0, 1, 2}, {1, 0, 2, 1}, {2, 0, 3, 1}}
	longUV  = [6][4]float32{{0, 0, 1, 14}, {0, 0, 1, 14}, {0, 0, 1, 14}, {0, 0, 1, 14}, {1, 0, 2, 1}, {2, 0, 3, 1}}
	pouchUV = [6][4]float32{{1, 1, 3, 2}, {6, 1, 8, 2}, {3, 1, 6, 2}, {8, 1, 11, 2}, {1, 2, 3, 5}, {3, 2, 5, 5}}
	gripUV  = [6][4]float32{{2, 6, 6, 10}, {8, 6, 12, 10}, {6, 6, 8, 10}, {12, 6, 14, 10}, {2, 10, 6, 12}, {6, 10, 10, 12}}

	// template_sling: two cords, the pouch and the grip
	slingRest = []cuboid{
		{mgl32.Vec3{6, 4, 7.5}, mgl32.Vec3{7, 6, 8.5}, cordUV},
		{mgl32.Vec3{9, 4, 7.5}, mgl32.Vec3{10, 6, 8.5}, cordUV},
		{mgl32.Vec3{7, 5, 6.5}, mgl32.Vec3{9, 6, 9.5}, pouchUV},
		{mgl32.Vec3{6, 0.02, 7}, mgl32.Vec3{10, 4, 9}, gripUV},
	}
	// template_sling_swinging: the cords at full length, the pouch at
	// their end
	slingSwinging = []cuboid{
		{mgl32.Vec3{6, 7, 7.5}, mgl32.Vec3{7, 21, 8.5}, longUV},
		{mgl32.Vec3{9, 7, 7.5}, mgl32.Vec3{10, 21, 8.5}, longUV},
		{mgl32.Vec3{7, 20, 6.5}, mgl32.Vec3{9, 21, 9.5}, pouchUV},
	}
)

// cuboidModel builds a block model from its elements, on 0..1 blocks,
// with Minecraft's face texturing: a face's (u1, v1) at its top left
// seen from outside (north's at +X, east's at +Z, up's at -Z, down's at +Z).
func cuboidModel(name string, els []cuboid, tex image.Image) *model.Model {
	m := bare(name, tex)
	var me model.Mesh
	for _, e := range els {
		lo, hi := e.from.Mul(1.0/16), e.to.Mul(1.0/16)
		w, h, d := hi.Sub(lo)[0], hi.Sub(lo)[1], hi.Sub(lo)[2]
		faces := [6]struct{ n, o, u, v mgl32.Vec3 }{ // normal, (u1, v1) corner, u and v directions
			{mgl32.Vec3{0, 0, -1}, mgl32.Vec3{hi[0], hi[1], lo[2]}, mgl32.Vec3{-w, 0, 0}, mgl32.Vec3{0, -h, 0}},
			{mgl32.Vec3{0, 0, 1}, mgl32.Vec3{lo[0], hi[1], hi[2]}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, -h, 0}},
			{mgl32.Vec3{1, 0, 0}, mgl32.Vec3{hi[0], hi[1], hi[2]}, mgl32.Vec3{0, 0, -d}, mgl32.Vec3{0, -h, 0}},
			{mgl32.Vec3{-1, 0, 0}, mgl32.Vec3{lo[0], hi[1], lo[2]}, mgl32.Vec3{0, 0, d}, mgl32.Vec3{0, -h, 0}},
			{mgl32.Vec3{0, 1, 0}, mgl32.Vec3{lo[0], hi[1], lo[2]}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, 0, d}},
			{mgl32.Vec3{0, -1, 0}, mgl32.Vec3{lo[0], lo[1], hi[2]}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, 0, -d}},
		}
		for i, f := range faces {
			uv := e.uv[i]
			base := uint32(len(me.Verts))
			for _, c := range [4][2]float32{{0, 0}, {0, 1}, {1, 1}, {1, 0}} { // counter-clockwise from outside
				p := f.o.Add(f.u.Mul(c[0])).Add(f.v.Mul(c[1]))
				u := (uv[0] + (uv[2]-uv[0])*c[0]) / 16
				v := (uv[1] + (uv[3]-uv[1])*c[1]) / 16
				me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: f.n, UV: mgl32.Vec2{u, v}, Weights: [4]float32{1}})
			}
			me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
		}
	}
	m.Meshes = []model.Mesh{me}
	return m
}

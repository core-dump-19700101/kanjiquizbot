package main

import (
        "image"
        "image/draw"
        "image/png"
        "io/ioutil"
        "log"
        "math"
        "bytes"

        "github.com/golang/freetype/truetype"
        "golang.org/x/image/font"
        "golang.org/x/image/math/fixed"
)

var (
        fontDpi      = 72.0 // font DPI setting
        fontFile = "meiryo.ttc" // TTF font filename
        fontHinting  = "full" // none | full
        fontSize     = 72.0 // font size in points
        fontTtf *truetype.Font
)

func init() {
        // Read the font data.
        fontBytes, err := ioutil.ReadFile(fontFile)
        if err != nil {
                log.Println(err)
                return
        }
        fontTtf, err = truetype.Parse(fontBytes)
        if err != nil {
                log.Println(err)
                return
        }
}

// Generate a PNG image reader with given string written
func GenerateImage(title string) *bytes.Buffer {

        h := font.HintingNone
        switch fontHinting {
        case "full":
                h = font.HintingFull
        }

        // Pick colours
        fg, bg := image.Black, image.White

        // Set up font drawer
        d := &font.Drawer{
                Src: fg,
                Face: truetype.NewFace(fontTtf, &truetype.Options{
                        Size:    fontSize,
                        DPI:     fontDpi,
                        Hinting: h,
                }),
        }

        // Create image canvas
        imgW := d.MeasureString(title).Round() * 11 / 10
        imgH := int(math.Ceil(fontSize*fontDpi/72 * 1.1))

        rgba := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

        // Draw the background and the guidelines
        draw.Draw(rgba, rgba.Bounds(), bg, image.ZP, draw.Src)

        // Attach image to font drawer
        d.Dst = rgba

        // Figure out writing position
        y := int(math.Ceil(fontSize*fontDpi/72 * 0.93))
        d.Dot = fixed.Point26_6{
                X: (fixed.I(imgW) - d.MeasureString(title)) / 2,
                Y: fixed.I(y),
        }

        // Write out the text
        d.DrawString(title)

        // Encode PNG image
        var buf bytes.Buffer
        err := png.Encode(&buf, rgba)
        if err != nil {
                log.Println(err)
                return &buf
        }

        return &buf
}

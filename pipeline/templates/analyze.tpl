ffmpeg -v quiet -i pipe:0 -f f32le -ac 1 -ar 44100 pipe:1 | bpm -m {{.Min}} -x {{.Max}} -f %0.0f

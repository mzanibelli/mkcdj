ffmpeg -v quiet -i pipe:0 -lavfi showwavespic=s=4096x2048:colors=#5294E2 -f image2 pipe:1

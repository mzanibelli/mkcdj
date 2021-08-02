ffmpeg -v quiet -i pipe:0 -lavfi showspectrumpic=s=4096x2048:color=cool:start=0:stop=24000 -f image2 pipe:1

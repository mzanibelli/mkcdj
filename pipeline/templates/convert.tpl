ffmpeg -v quiet -i pipe:0 -f wav -ac 2 -ar 44100 -acodec pcm_s16le pipe:1

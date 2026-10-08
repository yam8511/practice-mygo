# Builds my-app with GoCV against OpenCV 4.13.0 installed at C:\opencv.
$env:CGO_CXXFLAGS = "--std=c++11"
$env:CGO_CPPFLAGS = "-IC:/opencv/include"
$libs = "core","face","videoio","imgproc","highgui","imgcodecs","objdetect","features2d","video","dnn","xfeatures2d","plot","tracking","img_hash","calib3d","photo","aruco","wechat_qrcode","ximgproc","xphoto","bgsegm","bioinspired","ccalib"
$env:CGO_LDFLAGS = "-LC:/opencv/x64/mingw/lib " + (($libs | % { "-lopencv_${_}4130" }) -join " ")
$env:Path = "C:\opencv\x64\mingw\bin;$env:Path"   # runtime DLLs
go build -v

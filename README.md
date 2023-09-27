# Guitar-Flash-AutoPlay

Guitar flash autoplay v2 made with Golang. Previously it was built with Python.

This is a fun project. It may have bugs / errors. But %99 of the time it plays correctly.

Please test it on expert level.

Adjust these lines according to your screen resolution

Line: 20

```go
x, y, width, height := 0, 0, 1512, 982
```

Line: 32-36

```go
go pressNote(667, 673, "s", &previousReceiveTimeS, img)
go pressNote(760, 673, "j", &previousReceiveTimeJ, img)
go pressNote(866, 673, "k", &previousReceiveTimeK, img)
go pressNote(970, 673, "l", &previousReceiveTimeL, img)
```

Video Example


Feel Free to Contribute

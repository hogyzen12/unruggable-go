fyne package -os android -name Unruggable.apk -appID com.unruggable.app -icon icon.png &&
fyne package -os darwin -name Unruggable.app -appID com.unruggable.app -icon icon.png &&
fyne package -os iossimulator -appID com.unruggable.app -icon icon.png

fyne package -os ios -name Unruggable.app -appID com.unruggable.app -icon icon.png
fyne package -os windows -name Unruggable.apk -appID com.unruggable.app -icon icon.png

fyne release -os android -name Unruggable.apk -appID com.unruggable.app -icon icon.png -appVersion 1.0 -appBuild 1 -category utilities

fyne package -os web -appID com.unruggable.app -icon icon.png
fyne serve                                                   
Serving /Users/hogyzen12/coding-project-folders/unruggable-go/wasm on HTTP port: 8080
for wasm build then:
cp -r wasm/* /Users/hogyzen12/coding-project-folders/unruggable-web
copy everything to the web build
then:
cd /Users/hogyzen12/coding-project-folders/unruggable-web

and push to github, and let it autodeploy

tree -L 3 internal go.mod go.sum main.go > folders.txt
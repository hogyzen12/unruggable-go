fyne package -os android -name Unruggable.apk -appID com.unruggable.app -icon icon.png &&
fyne package -os darwin -name Unruggable.app -appID com.unruggable.app -icon icon.png &&
fyne package -os ios -name Unruggable.app -appID com.unruggable.app -icon icon.png

fyne package -os iossimulator -appID com.unruggable.app -icon icon.png
fyne package -os windows -name Unruggable.apk -appID com.unruggable.app -icon icon.png

fyne release -os android -name Unruggable.apk -appID com.unruggable.app -icon icon.png -appVersion 1.0 -appBuild 1 -category utilities
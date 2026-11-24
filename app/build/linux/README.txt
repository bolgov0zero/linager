linager-app is a single file: run it where it is. To have it in the menu of the desktop with its icon:

  mkdir -p ~/.local/bin ~/.local/share/applications ~/.local/share/icons/hicolor/256x256/apps
  cp linager-app ~/.local/bin/
  cp linager.png ~/.local/share/icons/hicolor/256x256/apps/
  sed "s|^Exec=.*|Exec=$HOME/.local/bin/linager-app|" linager.desktop > ~/.local/share/applications/linager.desktop

The tray needs a desktop with StatusNotifier icons (KDE; GNOME with the AppIndicator extension).

with import <nixpkgs> {};

let
  # External Script: This reads './dev.tmux' and puts it into a binary named 'dev-tmux'
  tmuxLayout   = pkgs.writeShellScriptBin "dev-tmux" (builtins.readFile ./dev.tmux);
in
pkgs.mkShell {

  name = "tmux-dev";
  nativeBuildInputs = with pkgs; [
    asciinema
    asciinema-agg
    go
    # git
    jq
    nodejs_22
    oh-my-zsh
    tmux
    tmuxLayout
    tree
  ];

  LANGUAGE = "Tmux";
  VERSION  = "tmux -V";

  shellHook = ''
    # Optional: Script environment start up
    echo "Welcome to $LANGUAGE Development Environment"
    $VERSION

    # The Trap: This kills the tmux session as soon as you exit the nix-shell
    # It ensures no "ghost" sessions stay running in the background.
    trap "tmux kill-session -t go-dev" EXIT

    # Perform Tmux Dev Layout
    exec dev-tmux
  '';
}

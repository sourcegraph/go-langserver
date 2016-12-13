![se7en](http://www.flutecrate.com/uploads/1/0/2/0/10200817/5243300_orig.jpg)

This directory contains scripts needed to generate (or update) Homebrew formula 
used to build, install, and test `go-langserver`. Also, you may find here the 
latest version of formula generated (`go-langserver.rb`)

# How to update formula

* Tag `go-langserver` with the new tag like `vX.Y.Z`
* Run `./generate.sh X.Y.Z`, it will compute all the things needed (SHA256 
checksum, Go dependencies and so on) and will update `formula.rb` with the new 
data based on `go-langserver.rb.template` template file.
* Copy resulting `go-langserver.rb` to Homebrew tap directory (ensure it has 
644 permissions) and run `brew audit --strict go-langserver`

Now you can submit PR to homebrew 
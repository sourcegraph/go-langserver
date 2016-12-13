require "language/go"

class GoLangserver < Formula
  desc "Go language LSP server"
  homepage "https://github.com/sourcegraph/go-langserver"
  url "https://github.com/sourcegraph/go-langserver/archive/v1.0.0.tar.gz"
  sha256 "e803e69f3df40e6e840d04f648e27ffcfb382ff77899c0c7a7b62cf3b3b277a4"

  head "https://github.com/sourcegraph/go-langserver.git"

  depends_on "go" => :build

  go_resource "github.com/beorn7/perks" do
    url "https://github.com/beorn7/perks.git",
      :revision => "4c0e84591b9aa9e6dcfdf3e020114cd81f89d5f9"
  end

  go_resource "github.com/gogo/protobuf" do
    url "https://github.com/gogo/protobuf.git",
      :revision => "6a92c871a8f5333cf106b2cdf937567208dbb2b7"
  end

  go_resource "github.com/golang/protobuf" do
    url "https://github.com/golang/protobuf.git",
      :revision => "8ee79997227bf9b34611aee7946ae64735e6fd93"
  end

  go_resource "github.com/matttproud/golang_protobuf_extensions" do
    url "https://github.com/matttproud/golang_protobuf_extensions.git",
      :revision => "c12348ce28de40eed0136aa2b644d0ee0650e56c"
  end

  go_resource "github.com/neelance/parallel" do
    url "https://github.com/neelance/parallel.git",
      :revision => "4de9ce63d14c18517a79efe69e10e99d32c850c3"
  end

  go_resource "github.com/opentracing/basictracer-go" do
    url "https://github.com/opentracing/basictracer-go.git",
      :revision => "1b32af207119a14b1b231d451df3ed04a72efebf"
  end

  go_resource "github.com/opentracing/opentracing-go" do
    url "https://github.com/opentracing/opentracing-go.git",
      :revision => "137bfcefd3340b28186f4fd3608719fcb120f98f"
  end

  go_resource "github.com/prometheus/client_golang" do
    url "https://github.com/prometheus/client_golang.git",
      :revision => "575f371f7862609249a1be4c9145f429fe065e32"
  end

  go_resource "github.com/prometheus/client_model" do
    url "https://github.com/prometheus/client_model.git",
      :revision => "fa8ad6fec33561be4280a8f0514318c79d7f6cb6"
  end

  go_resource "github.com/prometheus/common" do
    url "https://github.com/prometheus/common.git",
      :revision => "195bde7883f7c39ea62b0d92ab7359b5327065cb"
  end

  go_resource "github.com/prometheus/procfs" do
    url "https://github.com/prometheus/procfs.git",
      :revision => "abf152e5f3e97f2fafac028d2cc06c1feb87ffa5"
  end

  go_resource "github.com/slimsag/godocmd" do
    url "https://github.com/slimsag/godocmd.git",
      :revision => "a1005ad29fe3e4831773a8184ee7ebb3a41d1347"
  end

  go_resource "github.com/sourcegraph/ctxvfs" do
    url "https://github.com/sourcegraph/ctxvfs.git",
      :revision => "8e2bd62a565a647defd704af3d81fce05c5f190c"
  end

  go_resource "github.com/sourcegraph/jsonrpc2" do
    url "https://github.com/sourcegraph/jsonrpc2.git",
      :revision => "9fdd802ab4655d2258cef1b05c81be8e0fe4e2ad"
  end

  go_resource "golang.org/x/net" do
    url "https://go.googlesource.com/net.git",
      :revision => "4971afdc2f162e82d185353533d3cf16188a9f4e"
  end

  go_resource "golang.org/x/tools" do
    url "https://go.googlesource.com/tools.git",
      :revision => "e04df2157ae7263e17159baabadc99fb03fc7514"
  end

  def install
    mkdir_p buildpath/"src/github.com/sourcegraph"
    ln_s buildpath, buildpath/"src/github.com/sourcegraph/go-langserver"
    ENV["GOPATH"] = buildpath.to_s
    Language::Go.stage_deps resources, buildpath/"src"
    system "go", "build", "langserver/cmd/langserver-go/langserver-go.go"
    bin.install "langserver-go"
  end

  test do
    # Set up fake GOROOT and create test Go project
    mkdir_p testpath/"gopath/src/test"
    mkdir_p testpath/"goroot"
    ENV["GOPATH"] = "#{testpath}/gopath"
    ENV["GOROOT"] = "#{testpath}/goroot"
    (testpath/"gopath/src/test/p/a.go").write("package p; func A() {}")
    # Invoke initialize, hover, and finally exit requests and make sure that LSP server returns proper data
    init_req = "{\"id\":0,\"method\":\"initialize\",\"params\":{\"rootPath\":\"file://#{testpath}/gopath/src/test/p\"}}"
    init_res = "{\"id\":0,\"result\":{\"capabilities\":{\"textDocumentSync\":1,\"hoverProvider\":true,\"definitionProvider\":true,\"referencesProvider\":true,\"documentSymbolProvider\":true,\"workspaceSymbolProvider\":true}},\"jsonrpc\":\"2.0\"}"
    hover_req = "{\"id\":1,\"method\":\"textDocument/hover\",\"params\":{\"textDocument\":{\"uri\":\"file://#{testpath}/gopath/src/test/p/a.go\"},\"position\":{\"line\":0,\"character\":16}}}"
    hover_res = "{\"id\":1,\"result\":{\"contents\":[{\"language\":\"go\",\"value\":\"func A()\"}],\"range\":{\"start\":{\"line\":0,\"character\":16},\"end\":{\"line\":0,\"character\":17}}},\"jsonrpc\":\"2.0\"}"
    exit_req = "{\"id\":2,\"method\":\"exit\",\"params\":{}}"
    require "open3"
    Open3.popen3("langserver-go") do |stdin, stdout, _|
      stdin.write("Content-Length: #{init_req.length}\r\n\r\n#{init_req}")
      sleep(1)
      stdin.write("Content-Length: #{hover_req.length}\r\n\r\n#{hover_req}")
      sleep(1)
      stdin.write("Content-Length: #{exit_req.length}\r\n\r\n#{exit_req}")
      stdin.close
      assert_equal "Content-Length: #{init_res.length}\r\nContent-Type: application/vscode-jsonrpc; charset=utf8\r\n\r\n#{init_res}Content-Length: #{hover_res.length}\r\nContent-Type: application/vscode-jsonrpc; charset=utf8\r\n\r\n#{hover_res}", stdout.read
    end
  end
end

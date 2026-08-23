class Dnser < Formula
  desc "Local DNS management for development"
  homepage "https://github.com/SDK-E/dnser"
  version "1.0.0"
  license :"business-source-license"

  if Hardware::CPU.arm?
    url "https://github.com/SDK-E/dnser/releases/download/v#{version}/dnser_#{version}_Darwin_arm64.tar.gz"
    sha256 :no_check
  else
    url "https://github.com/SDK-E/dnser/releases/download/v#{version}/dnser_#{version}_Darwin_amd64.tar.gz"
    sha256 :no_check
  end

  def install
    bin.install "dnser"
  end

  def caveats
    <<~EOS
      Run the guided setup once:
        dnser setup
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dnser version")
  end
end

class Qc < Formula
  desc "Quick Commerce CLI for Zepto and Blinkit — OTP auth, live search, price comparison & spend analytics"
  homepage "https://github.com/4nkitd/quick-commerce-cli"
  version "0.2.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/4nkitd/quick-commerce-cli/releases/download/v0.2.0/qc_v0.2.0_darwin_arm64.tar.gz"
      sha256 "8084a194408bafa4ffa8c914da9580541394d471fabc93f321165a1a380214ed"
    else
      url "https://github.com/4nkitd/quick-commerce-cli/releases/download/v0.2.0/qc_v0.2.0_darwin_amd64.tar.gz"
      sha256 "97d77bbfb7d04a6e4789cdd1d46e68cff5ae518f370bc1e0859946cd7a699e91"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/4nkitd/quick-commerce-cli/releases/download/v0.2.0/qc_v0.2.0_linux_arm64.tar.gz"
      sha256 "6081f285b8fd3b7d2ab4bfe8d640e1579076debb811f71db8b2f79f38400b7d7"
    else
      url "https://github.com/4nkitd/quick-commerce-cli/releases/download/v0.2.0/qc_v0.2.0_linux_amd64.tar.gz"
      sha256 "0165cf9c115ee06f3180872a0924ea54f714db3a6fc8d552e250e4d7537ff42f"
    end
  end

  def install
    bin.install "qc"
  end

  test do
    assert_match "qc version v0.2.0", shell_output("#{bin}/qc --version")
  end
end

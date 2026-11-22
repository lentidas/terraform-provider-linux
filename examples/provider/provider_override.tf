resource "linux_script" "install_package" {
  provider_override {
    id = "myhost"

    host     = "127.0.0.1"
    port     = 22
    user     = "root"
    password = "root"
  }

  lifecycle_commands {
    create = "apt update && apt install -y $PACKAGE_NAME=$PACKAGE_VERSION"
    read   = "apt-cache policy $PACKAGE_NAME | grep 'Installed:' | grep -v '(none)' | awk '{ print $2 }' | xargs | tr -d '\n'"
    update = "apt update && apt install -y $PACKAGE_NAME=$PACKAGE_VERSION"
    delete = "apt remove -y $PACKAGE_NAME"
  }

  environment = {
    PACKAGE_NAME    = "apache2"
    PACKAGE_VERSION = "2.4.18-2ubuntu3.4"
  }
}

data "linux_script" "package_version" {
  lifecycle_commands {
    read = "apt-cache policy $PACKAGE_NAME | grep 'Installed:' | grep -v '(none)' | awk '{ print $2 }' | xargs | tr -d '\n'"
  }

  environment = {
    PACKAGE_NAME = "apache2"
  }
}

resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t2.micro"
}
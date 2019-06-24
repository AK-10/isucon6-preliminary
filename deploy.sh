cd ~/webapp/go
make
sudo systemctl restart nginx
sudo systemctl restart isuda.go
cd ~/isucon6q
./isucon6q-bench
cd ~

# Ожидаемый результат: `ebuild-updates.txt`

Это читаемая спецификация фактов для fixture, а не окончательный JSON API. Ссылки указывают номера строк исходного файла.

## Записи

| № | Строка | Пакет | Операция и подтверждённый контекст |
|---:|---:|---|---|
| 1 | 6 | `dev-libs/libei` | `upgrade`; `1.6.0` → `1.6.0-r1`; репозиторий `gentoo` до и после. |
| 2 | 7 | `app-admin/chezmoi` | `upgrade`; `2.73.3` → `2.74.0`; репозиторий `gentoo-zh` до и после; целевая версия помечена `~`. |
| 3 | 8 | `app-emulation/qemu` | `upgrade`; `10.2.3-r3` → `10.2.3-r4`; репозиторий `gentoo` до и после. |
| 4 | 9 | `app-editors/vscode` | `upgrade`; `1.144.0` → `1.144.0-r1`; репозиторий `gentoo` до и после. |
| 5 | 10 | `sys-kernel/gentoo-kernel-bin` | `install` в новый SLOT; целевая версия и SLOT `6.18.60`, репозиторий `gentoo`. Показанный установленный контекст: версия `6.18.57`, SLOT `6.18.57`, репозиторий `gentoo`. Это не свидетельствует о замене или удалении старой версии; `before` не означает наличие пакета в целевом SLOT. |
| 6 | 11 | `virtual/dist-kernel` | `upgrade`; `6.18.57` → `6.18.60`; репозиторий `gentoo` до и после; SLOT `0` → `0`; SUBSLOT `6.18.57` → `6.18.60`. |

## Группы флагов

Ниже приведено точное содержимое групп из соответствующей строки. Скобки, регистр и маркеры сохранены. Отсутствующая группа не выводится как пустой или известный список.

| № | Группа | Значения и маркеры |
|---:|---|---|
| 1 | `USE` | `(systemd) (-elogind) -test` |
| 2 | — | Группы не представлены. |
| 3 | `USE` | `(X) aio bpf bzip2 capstone curl fdt filecaps gnutls io-uring jpeg ncurses nls opengl pam passt pin-upstream-blobs pipewire png python seccomp slirp spice udev usb usbredir verify-sig vhost-net virgl virtfs vnc wayland xattr zstd -accessibility -alsa -debug -doc -fuse -glusterfs -gtk -infiniband -iscsi -jack -keyutils -lzo -multipath -nfs -numa -oss -plugins -pulseaudio -rbd -sasl -sdl -sdl-image (-selinux) -smartcard -snappy -ssh -static-user -systemtap -test -valgrind -vde -vte -xdp -xen` |
| 3 | `PYTHON_TARGETS` | `python3_14 -python3_13` |
| 3 | `QEMU_SOFTMMU_TARGETS` | `x86_64 -aarch64 -alpha -arm -avr -hppa -i386 -loongarch64 -m68k -microblaze -microblazeel -mips -mips64 -mips64el -mipsel -or1k -ppc -ppc64 -riscv32 -riscv64 -rx -s390x -sh4 -sh4eb -sparc -sparc64 -tricore -xtensa -xtensaeb` |
| 3 | `QEMU_USER_TARGETS` | `-aarch64 -aarch64_be -alpha -arm -armeb -hexagon -hppa -i386 -loongarch64 -m68k -microblaze -microblazeel -mips -mips64 -mips64el -mipsel -mipsn32 -mipsn32el -or1k -ppc -ppc64 -ppc64le -riscv32 -riscv64 -s390x -sh4 -sh4eb -sparc -sparc32plus -sparc64 -x86_64 -xtensa -xtensaeb` |
| 4 | `USE` | `egl wayland -webkit` |
| 4 | `L10N` | `af am ar bg bn ca cs da de el en-GB es es-419 et fa fi fil fr gu he hi hr hu id it ja kn ko lt lv ml mr ms nb nl pl pt-BR pt-PT ro ru sk sl sr sv sw ta te th tr uk ur vi zh-CN zh-TW` |
| 5 | `USE` | `initramfs verify-sig -debug (-generic-uki) -modules-compress -test` |
| 6 | — | Группы не представлены. |

Отсутствие группы у записей 2 и 6 фиксирует только отсутствие блока в исходной строке. Маркер `~` у записи 2 сохраняется отдельно от групп флагов и не задаёт severity.

## Размеры и сводка

| № | Строка | Размер скачивания |
|---:|---:|---:|
| 1 | 6 | `0 KiB` |
| 2 | 7 | `11897 KiB` |
| 3 | 8 | `0 KiB` |
| 4 | 9 | `0 KiB` |
| 5 | 10 | `469316 KiB` |
| 6 | 11 | `0 KiB` |

Сводка из строки 13: `6 packages`, `5 upgrades`, `1 in new slot`, размер скачивания `481212 KiB`.

## Статус и диагностики

Для полного неизменённого fixture ожидается статус `parsed`, шесть записей в указанном порядке и отсутствие диагностик. Заголовки, строки расчёта зависимостей, сводка и приглашение Portage распознаются как допустимый служебный текст. Приглашение `Would you like to merge these packages? [Yes/No]` не является approval пользователя сервиса.

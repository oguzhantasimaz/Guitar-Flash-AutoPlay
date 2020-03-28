import keyboard
from time import sleep
import numpy as np
from PIL import ImageGrab
from threading import Thread

#OĞUZHAN TAŞIMAZ 29.03.2020


y = 65  # 952
greenx = 58  # 782
redx = 147  # 864
yellowx = 237  # 950
bluex = 325  # 1030
orangex = 417  # 1118
basecolor = 75

def green(image, greenx, y, basecolor):
    greena = np.all(image.getpixel((greenx, y)) >= basecolor)
    if greena:
        keyboard.press_and_release('a')


def red(image, redx, y, basecolor):
    reda = np.all(image.getpixel((redx, y)) >= basecolor)
    if reda:
        keyboard.press_and_release('s')


def yellow(image, yellowx, y, basecolor):
    yellowa = np.all(image.getpixel((yellowx, y)) >= basecolor)
    if yellowa:
        keyboard.press_and_release('j')

def blue(image, bluex, y, basecolor):
    bluea = np.all(image.getpixel((bluex, y)) >= basecolor)
    if bluea:
        keyboard.press_and_release('k')


def orange(image, orangex, y, basecolor):
    orangea = np.all(image.getpixel((orangex, y)) >= basecolor)
    if orangea:
        keyboard.press_and_release('l')


while True:
    image = ImageGrab.grab(bbox=(710, 866, 1210, 1000)).convert('L')
    Thread(green(image, greenx, y, basecolor)).start()
    Thread(red(image, redx, y, basecolor)).start()
    Thread(yellow(image, yellowx, y, basecolor)).start()
    Thread(blue(image, bluex, y, basecolor)).start()
    Thread(orange(image, orangex, y, basecolor)).start()

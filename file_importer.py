import os
from book_file_processors import BookFileProcessorFactory, UnsupportedBookFileType


class BookFileImporter(object):
    dao = None
    BookFileProcessorFactory = BookFileProcessorFactory

    def __init__(self, path):
        self.path = os.path.abspath(path)

    def try_to_import_file_if_not_already_imported(self):
        if self.is_file_already_imported():
            return
        try:
            book = self.get_book_file_processor().get_book()
        except UnsupportedBookFileType:
            return
        self.save_book(book)

    def save_book(self, book):
        self.dao.save_book(book)

    def is_file_already_imported(self):
        return self.dao.book_with_path_already_exists(self.path)

    def get_book_file_processor(self):
        return self.BookFileProcessorFactory(self.path).get_book_file_processor()

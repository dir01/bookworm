class Book(object):
    def __init__(self, **kwargs):
        self.__dict__.update(kwargs)

    def __repr__(self):
        return '<Book: %s - %s>' % (
            self.get_author_full_name().encode('utf-8'),
            self.title.encode('utf-8')
        )

    def get_author_full_name(self):
        return ' '.join([
            self.author_first_name,
            self.author_middle_name,
            self.author_last_name,
        ])